//go:build linux

package systemupdate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

// These are installation scratch paths, NOT the installed engines, their
// configurations, log history, certificates, keys, or rollback snapshots.
func isProtocolStagingFile(name string) bool {
	if name == ".caddy-naive.new" || name == "telemt.new" {
		return true
	}
	for _, prefix := range []string{
		".hysteria-", ".pingtunnel-", ".trusttunnel_endpoint-",
		".openflux-", ".vetroff-", ".sudoku-tunnel-",
	} {
		if strings.HasPrefix(name, prefix) {
			// A failed OpenFlux replacement may have left its only backup.
			return !strings.HasPrefix(name, ".openflux-previous-")
		}
	}
	return false
}

func isProtocolConfigStagingFile(name string) bool {
	return strings.HasPrefix(name, ".oxidns-") ||
		strings.HasPrefix(name, ".dnstt-") ||
		(strings.HasPrefix(name, ".telemt-") &&
			(strings.HasSuffix(name, ".toml") || strings.HasSuffix(name, ".env")))
}

func isProtocolTempFile(name string) bool {
	for _, prefix := range []string{
		"olcrtc-installer-", "olcrtc-built-revisions-", "olcrtc-install-log-",
	} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isAbandonedInstallerDir(name string) bool {
	for _, prefix := range []string{
		"3x-ui-sudoku-", "3x-ui-externalvpn-", "3x-ui-fptn-client-",
		"3x-ui-caddy-naive-", "3x-ui-naive-check-",
		"olcrtc-patches-", "3x-ui-olcrtc.", "telemt-update.",
	} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func isAbandonedPanelStageDir(name string) bool {
	return strings.HasPrefix(name, ".core-stage.") ||
		strings.HasPrefix(name, ".x-ui-release.")
}

func isOldPanelDownload(name string) bool {
	if strings.HasPrefix(name, ".x-ui-download.") {
		for _, suffix := range []string{".tar.gz", ".tar.gz.part", ".tar.gz.sha256", ".tar.gz.sha256.part"} {
			if strings.HasSuffix(name, suffix) {
				return true
			}
		}
	}
	for _, exact := range []string{
		"x-ui-linux-amd64.tar.gz", "x-ui-linux-arm64.tar.gz",
		"x-ui-linux-amd64.tar.gz.part", "x-ui-linux-arm64.tar.gz.part",
		"x-ui-linux-amd64.tar.gz.sha256", "x-ui-linux-arm64.tar.gz.sha256",
	} {
		if name == exact {
			return true
		}
	}
	return false
}

// Rollback copies are deliberately never removed by generic cache cleanup.
// The owner-specific recovery code must restore or discard them first.
func mustPreserveRecoveryDir(parent, name string) bool {
	switch {
	case strings.HasPrefix(name, ".vetroff-stage-"):
		_, err := os.Lstat(filepath.Join(parent, name, "previous"))
		return err == nil || !errors.Is(err, os.ErrNotExist)
	case strings.HasPrefix(name, "telemt-update."):
		for _, backup := range []string{"previous-telemt", "previous-version"} {
			if _, err := os.Lstat(filepath.Join(parent, name, backup)); !errors.Is(err, os.ErrNotExist) {
				return true
			}
		}
	case strings.HasPrefix(name, "3x-ui-olcrtc."):
		if _, err := os.Lstat("/etc/olcrtc-manager/.3x-ui-transaction"); !errors.Is(err, os.ErrNotExist) {
			return true
		}
	}
	return false
}

// removeOldKnownDirectories only removes explicitly recognized and aged
// installer scratch directories, never a backup/transaction or symlink.
func removeOldKnownDirectories(
	ctx context.Context,
	parent string,
	minAge time.Duration,
	match func(string) bool,
	result *CleanupResult,
) error {
	info, err := os.Lstat(parent)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing unsafe cleanup directory: %s", parent)
	}
	entries, err := os.ReadDir(parent)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() || !match(entry.Name()) || mustPreserveRecoveryDir(parent, entry.Name()) {
			continue
		}
		path := filepath.Join(parent, entry.Name())
		stat, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 || time.Since(stat.ModTime()) < minAge {
			continue
		}
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("remove abandoned installer directory %s: %w", path, err)
		}
		result.RemovedFiles++
	}
	return nil
}

// tryUpdateFileLock holds an existing updater's advisory lock during panel
// artifact removal. Missing lock files are harmless; no new lock is created.
func tryUpdateFileLock(path string) (func(), bool, error) {
	stat, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return func() {}, true, nil
	}
	if err != nil {
		return nil, false, err
	}
	if !stat.Mode().IsRegular() {
		return nil, false, fmt.Errorf("unexpected updater lock type: %s", path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, true, nil
}

func cleanupPanelUpdateArtifacts(ctx context.Context, binDir string, result *CleanupResult) error {
	// The normal layout is <installation>/bin; do not guess custom layouts.
	if filepath.Base(binDir) != "bin" {
		return nil
	}
	installation := filepath.Dir(binDir)
	if installation == "/" || installation == "." {
		return nil
	}
	parent := filepath.Dir(installation)
	for _, lock := range []string{
		filepath.Join(parent, ".x-ui-download.lock"),
		filepath.Join(parent, ".x-ui-upgrade.lock"),
	} {
		unlock, available, err := tryUpdateFileLock(lock)
		if err != nil {
			return fmt.Errorf("panel update lock: %w", err)
		}
		if !available {
			return fmt.Errorf("panel/core update in progress; preserved its download and staging files")
		}
		defer unlock()
	}
	if err := removeStaleFiles(ctx, parent, tempArtifactMaxAge, isOldPanelDownload, result); err != nil {
		return err
	}
	if err := removeOldKnownDirectories(ctx, parent, tempArtifactMaxAge, func(name string) bool {
		return strings.HasPrefix(name, ".x-ui-release.")
	}, result); err != nil {
		return err
	}
	return removeOldKnownDirectories(ctx, installation, tempArtifactMaxAge, isAbandonedPanelStageDir, result)
}

func cleanupProtocolInstallArtifacts(ctx context.Context, result *CleanupResult) {
	binDir := config.GetBinFolderPath()
	clean := func(dir string, maxAge time.Duration, match func(string) bool) {
		if err := removeStaleFiles(ctx, dir, maxAge, match, result); err != nil {
			result.Warnings = append(result.Warnings, err.Error())
		}
	}
	clean(binDir, tempArtifactMaxAge, isProtocolStagingFile)
	// All 3x-ui-managed standalone protocol binaries write only to these
	// private locations. Real settings, keys and state files don't match.
	for _, dir := range []string{
		"/etc/x-ui/oxidns", "/etc/x-ui/oxidns/bin",
		"/etc/x-ui/dnstt", "/etc/x-ui/dnstt/bin",
		"/etc/x-ui", filepath.Join(binDir, "mtproto"),
	} {
		clean(dir, tempArtifactMaxAge, isProtocolConfigStagingFile)
	}
	for _, dir := range []string{"/tmp", "/var/tmp"} {
		clean(dir, tempArtifactMaxAge, isProtocolTempFile)
	}
	// These fixed names are used for interrupted atomic Telemt/OlcRTC writes.
	clean("/usr/local/x-ui/bin", tempArtifactMaxAge, func(name string) bool {
		return name == "telemt.new"
	})
	clean("/usr/local/x-ui", tempArtifactMaxAge, func(name string) bool {
		return name == "telemt-update.sh.new"
	})
	clean("/etc/x-ui", tempArtifactMaxAge, func(name string) bool {
		return name == "telemt.version.new"
	})

	if err := removeOldKnownDirectories(ctx, binDir, tempArtifactMaxAge, func(name string) bool {
		return strings.HasPrefix(name, ".vetroff-stage-")
	}, result); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	if err := cleanupPanelUpdateArtifacts(ctx, binDir, result); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	if err := cleanupUnusedOlcRTCCache(ctx, result); err != nil {
		result.Warnings = append(result.Warnings, err.Error())
	}
	result.Actions = append(result.Actions, "Abandoned standalone VPN component downloads and installers")
}

// The OlcRTC Go toolchain and build caches can be several GB. Remove them
// only after the managed service has been completely uninstalled; never drop
// clients, credentials, transaction journals, or a running build.
func cleanupUnusedOlcRTCCache(ctx context.Context, result *CleanupResult) error {
	for _, keep := range []string{
		"/usr/local/bin/olcrtc",
		"/usr/local/bin/olcrtc-manager",
		"/etc/systemd/system/olcrtc-manager.service",
		"/etc/olcrtc-manager/.3x-ui-revisions",
		"/etc/olcrtc-manager/.3x-ui-transaction",
	} {
		if _, err := os.Lstat(keep); !errors.Is(err, os.ErrNotExist) {
			return nil
		}
	}
	// A Go compiler/download job might be continuing a prior installation.
	for _, process := range []string{"go", "compile", "link", "git", "curl", "tar"} {
		active, err := activeNamedProcess("/proc", process)
		if err != nil || active {
			return nil
		}
	}
	for _, root := range []string{"/opt/3x-ui/olcrtc/cache", "/opt/3x-ui/olcrtc/toolchains"} {
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := os.Lstat(root)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || time.Since(info.ModTime()) < 48*time.Hour {
			continue
		}
		if err := os.RemoveAll(root); err != nil {
			return fmt.Errorf("remove unused OlcRTC cache %s: %w", root, err)
		}
		result.RemovedFiles++
		result.Actions = append(result.Actions, "Unused OlcRTC Go build and toolchain cache")
	}
	return nil
}

func activeNamedProcess(procDir, process string) (bool, error) {
	entries, err := os.ReadDir(procDir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		comm, err := os.ReadFile(filepath.Join(procDir, entry.Name(), "comm"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(string(comm)) == process {
			return true, nil
		}
	}
	return false, nil
}
