package job

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

const (
	defaultLogRetentionDays = 7
	maxLogRetentionDays     = 90
)

type archivedLogSource struct {
	label string
	path  string
}

// Configured duration applies to panel rotations, core daily snapshots and
// core console rotations. Database-less tools/tests use the safe default.
func configuredLogRetentionDays() int {
	if database.GetDB() == nil {
		return defaultLogRetentionDays
	}
	days, err := (&service.SettingService{}).GetLogRetentionDays()
	if err != nil || days < 1 || days > maxLogRetentionDays {
		return defaultLogRetentionDays
	}
	return days
}

func logArchiveDir() string {
	return filepath.Join(config.GetLogFolder(), "core-log-archives")
}

// archiveManagedCoreLogs is called once per day. Unlike the previous daily
// wipe, a snapshot remains available for the configured retention period.
// Only files expressly managed by the panel are considered.
func archiveManagedCoreLogs(now time.Time) {
	sources := []archivedLogSource{
		{label: "xray-access", path: readableLogPath(xray.GetAccessLogPath)},
		{label: "xray-error", path: readableLogPath(xray.GetErrorLogPath)},
		{label: "sing-box", path: filepath.Join(config.GetBinFolderPath(), "sing-box.log")},
		{label: "sing-box-upstream", path: filepath.Join(config.GetBinFolderPath(), "sing-box-upstream.log")},
	}
	if path, err := coreLogPath(); err == nil && !disabledLogPath(path) {
		sources = append(sources, archivedLogSource{label: "native-core", path: path})
	}
	servers := filepath.Join(config.GetBinFolderPath(), "protocol-servers")
	if dirs, err := os.ReadDir(servers); err == nil {
		for _, dir := range dirs {
			if dir.IsDir() {
				sources = append(sources, archivedLogSource{
					label: "protocol-" + safeArchiveLabel(dir.Name()),
					path:  filepath.Join(servers, dir.Name(), "server.log"),
				})
			}
		}
	}
	seen := make(map[string]bool)
	for _, source := range sources {
		if disabledLogPath(source.path) {
			continue
		}
		path, err := filepath.Abs(source.path)
		if err != nil || seen[path] {
			continue
		}
		seen[path] = true
		if err := archiveLogFile(path, source.label, now); err != nil {
			logger.Warning("Could not archive core log:", path, "-", err)
		}
	}
	pruneLogArchives(now, configuredLogRetentionDays())
}

func readableLogPath(fn func() (string, error)) string {
	path, err := fn()
	if err != nil {
		return ""
	}
	return path
}

func safeArchiveLabel(s string) string {
	var b strings.Builder
	for _, r := range s {
		if b.Len() >= 64 {
			break
		}
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' ||
			r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func archiveLogFile(path, label string, now time.Time) error {
	// Do not follow symlinks or archive devices and sockets.
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return nil
	}
	src, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer src.Close()
	info, err = src.Stat()
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return nil
	}
	dir := logArchiveDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, ".log-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	from := max(int64(0), info.Size()-maxXrayLogBytes)
	gz := gzip.NewWriter(temp)
	_, copyErr := io.Copy(gz, io.NewSectionReader(src, from, info.Size()-from))
	closeErr := gz.Close()
	if copyErr == nil {
		copyErr = closeErr
	}
	if copyErr == nil {
		copyErr = temp.Sync()
	}
	if err := temp.Close(); err != nil && copyErr == nil {
		copyErr = err
	}
	if copyErr != nil {
		return copyErr
	}
	filename := fmt.Sprintf("%s-%s.log.gz", safeArchiveLabel(label),
		now.UTC().Format("20060102T150405.000000000Z"))
	target := filepath.Join(dir, filename)
	if err := os.Rename(temp.Name(), target); err != nil {
		return err
	}
	// Preserve the inode to allow the already running core to keep writing.
	// Archive first; never truncate if the snapshot could not be persisted.
	return src.Truncate(0)
}

// Time pruning never touches active log files; only known archive/rotated
// segment names are eligible. Size caps are enforced by the existing pruner.
func pruneLogArchives(now time.Time, days int) {
	if days < 1 || days > maxLogRetentionDays {
		days = defaultLogRetentionDays
	}
	cutoff := now.Add(-time.Duration(days) * 24 * time.Hour)
	pruneNamedLogFiles(logArchiveDir(), cutoff, func(name string) bool {
		return strings.HasSuffix(name, ".log.gz") &&
			(strings.HasPrefix(name, "xray-") ||
				strings.HasPrefix(name, "sing-box-") ||
				strings.HasPrefix(name, "sing-box_") ||
				strings.HasPrefix(name, "native-core-") ||
				strings.HasPrefix(name, "protocol-"))
	})
	pruneNamedLogFiles(config.GetLogFolder(), cutoff, func(name string) bool {
		return strings.HasPrefix(name, "3xui-") &&
			(strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.gz"))
	})
	pruneNamedLogFiles(config.GetBinFolderPath(), cutoff, func(name string) bool {
		return (strings.HasPrefix(name, "hiddify-core") ||
			strings.HasPrefix(name, "sing-box") ||
			strings.HasPrefix(name, "xray-")) &&
			strings.Contains(name, ".console-") &&
			(strings.HasSuffix(name, ".log") || strings.HasSuffix(name, ".log.gz"))
	})
}

func pruneNamedLogFiles(dir string, cutoff time.Time, allowed func(string) bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warning("Could not scan log archives:", dir, "-", err)
		}
		return
	}
	for _, entry := range entries {
		if !allowed(entry.Name()) || !entry.Type().IsRegular() {
			continue
		}
		info, err := entry.Info()
		if err != nil || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !os.IsNotExist(err) {
			logger.Warning("Could not remove expired log archive:", entry.Name(), "-", err)
		}
	}
}
