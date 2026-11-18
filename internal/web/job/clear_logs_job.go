package job

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

const defaultMaxXrayLogBytes int64 = 64 << 20

var maxXrayLogBytes = defaultMaxXrayLogBytes

// ClearLogsJob clears old log files to prevent disk space issues.
type ClearLogsJob struct{}

// PruneXrayLogsJob bounds both legacy and current core logs, and IP-limit logs.
type PruneXrayLogsJob struct{}

// NewClearLogsJob creates a new log cleanup job instance.
func NewClearLogsJob() *ClearLogsJob {
	return new(ClearLogsJob)
}

// NewPruneXrayLogsJob creates a new Xray log pruning job instance.
func NewPruneXrayLogsJob() *PruneXrayLogsJob {
	return new(PruneXrayLogsJob)
}

// ensureFileExists creates the necessary directories and file if they don't exist
func ensureFileExists(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	file.Close()
	return nil
}

// Here Run is an interface method of the Job interface
func (j *ClearLogsJob) Run() {
	logFiles := []string{xray.GetIPLimitLogPath(), xray.GetIPLimitBannedLogPath()}
	logFilesPrev := []string{xray.GetIPLimitBannedPrevLogPath()}

	// Ensure all log files and their paths exist
	for _, path := range append(logFiles, logFilesPrev...) {
		if err := ensureFileExists(path); err != nil {
			logger.Warning("Failed to ensure log file exists:", path, "-", err)
		}
	}

	// Clear log files and copy to previous logs
	for i := range len(logFiles) {
		if i > 0 {
			// Copy to previous logs
			logFilePrev, err := os.OpenFile(logFilesPrev[i-1], os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
			if err != nil {
				logger.Warning("Failed to open previous log file for writing:", logFilesPrev[i-1], "-", err)
				continue
			}

			logFile, err := os.OpenFile(logFiles[i], os.O_RDONLY, 0o644)
			if err != nil {
				logger.Warning("Failed to open current log file for reading:", logFiles[i], "-", err)
				logFilePrev.Close()
				continue
			}

			// A pre-existing multi-GB log must not be duplicated at midnight.
			// Keep only its bounded tail in the previous-day snapshot.
			info, statErr := logFile.Stat()
			if statErr == nil {
				start := max(int64(0), info.Size()-maxXrayLogBytes)
				_, err = io.Copy(logFilePrev, io.NewSectionReader(logFile, start, info.Size()-start))
			} else {
				err = statErr
			}
			if err != nil {
				logger.Warning("Failed to copy log file:", logFiles[i], "to", logFilesPrev[i-1], "-", err)
			}

			logFile.Close()
			closeErr := logFilePrev.Close()
			if err != nil || closeErr != nil {
				// Preserve the source when the snapshot could not be written.
				continue
			}
		}

		err := os.Truncate(logFiles[i], 0)
		if err != nil {
			logger.Warning("Failed to truncate log file:", logFiles[i], "-", err)
		}
	}

	wipeXrayLogs()
}

func (j *PruneXrayLogsJob) Run() {
	truncateXrayLog(xray.GetAccessLogPath, maxXrayLogBytes)
	truncateXrayLog(xray.GetErrorLogPath, maxXrayLogBytes)
	truncateXrayLog(coreLogPath, maxXrayLogBytes)
	for _, path := range []string{xray.GetIPLimitLogPath(), xray.GetIPLimitBannedLogPath(), xray.GetIPLimitBannedPrevLogPath()} {
		truncateLogFile(path, maxXrayLogBytes)
	}
	truncateProtocolServerLogs(maxXrayLogBytes)
}

func wipeXrayLogs() {
	truncateXrayLog(xray.GetAccessLogPath, 0)
	truncateXrayLog(xray.GetErrorLogPath, 0)
	truncateXrayLog(coreLogPath, 0)
	truncateProtocolServerLogs(0)
}

func truncateProtocolServerLogs(maxBytes int64) {
	dir := filepath.Join(config.GetBinFolderPath(), "protocol-servers")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warning("Failed to list protocol server logs:", err)
		}
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			truncateLogFile(filepath.Join(dir, entry.Name(), "server.log"), maxBytes)
		}
	}
}

// Read the actual output setting: templates may change it or disable logging.
// Relative outputs are resolved against the core's working directory.
func coreLogPath() (string, error) {
	data, err := os.ReadFile(hiddify.GetConfigPath())
	if err != nil {
		return "", err
	}
	var cfg struct {
		Log struct {
			Disabled bool   `json:"disabled"`
			Output   string `json:"output"`
		} `json:"log"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return "", err
	}
	if cfg.Log.Disabled || cfg.Log.Output == "" {
		return "", nil
	}
	if filepath.IsAbs(cfg.Log.Output) {
		return cfg.Log.Output, nil
	}
	return filepath.Join(filepath.Dir(hiddify.GetBinaryPath()), cfg.Log.Output), nil
}

func truncateXrayLog(pathFn func() (string, error), maxBytes int64) {
	logPath, err := pathFn()
	if err != nil || disabledLogPath(logPath) {
		return
	}
	truncateLogFile(logPath, maxBytes)
}

func truncateLogFile(logPath string, maxBytes int64) {
	info, err := os.Stat(logPath)
	if err != nil {
		if !os.IsNotExist(err) {
			logger.Warning("Failed to stat core log:", logPath, "-", err)
		}
		return
	}
	// Never truncate devices such as /dev/stdout, or pipes/directories.
	if !info.Mode().IsRegular() || (maxBytes > 0 && info.Size() <= maxBytes) {
		return
	}
	if err := os.Truncate(logPath, 0); err != nil && !os.IsNotExist(err) {
		logger.Warning("Failed to truncate core log:", logPath, "-", err)
	}
}

func disabledLogPath(path string) bool {
	return path == "" || path == "none"
}
