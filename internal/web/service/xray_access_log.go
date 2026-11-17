package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

// selectedXrayAccessLogPath resolves the access log of the currently selected
// Xray process. Standalone Xray runs with xray.json; the old in-process Xray
// setup uses config.json. Never read a stale legacy log for standalone Xray.
func selectedXrayAccessLogPath() (string, error) {
	// The log reader also serves diagnostics before database initialization.
	if database.GetDB() == nil {
		return xray.GetAccessLogPath()
	}
	core, err := (&SettingService{}).GetCoreType()
	if err != nil || core != CoreTypeXray {
		return xray.GetAccessLogPath()
	}

	configPath := NativeCore(CoreTypeXray).ProcessConfigPath()
	raw, err := os.ReadFile(configPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	path, err := parseXrayAccessLogConfig(raw)
	if err != nil {
		return "", err
	}
	return resolveXrayAccessLogPath(path, configPath), nil
}

func parseXrayAccessLogConfig(raw []byte) (string, error) {
	var cfg struct {
		Log struct {
			Access string `json:"access"`
		} `json:"log"`
	}
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return "", err
	}
	return cfg.Log.Access, nil
}

// resolveXrayAccessLogPath uses the process working directory, not the
// panel service's directory, for Xray's relative access-log filenames.
func resolveXrayAccessLogPath(logPath, configPath string) string {
	logPath = strings.TrimSpace(logPath)
	if logPath == "" || logPath == "none" || filepath.IsAbs(logPath) {
		return logPath
	}
	return filepath.Clean(filepath.Join(filepath.Dir(configPath), logPath))
}
