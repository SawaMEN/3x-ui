package service

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

// selectedXrayAccessLogPath resolves the access log of the currently selected
// Xray process. Standalone Xray runs with xray.json; the old in-process Xray
// setup uses config.json. Never read a stale legacy log for standalone Xray.
func selectedXrayAccessLogPath() (string, error) {
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
	return parseXrayAccessLogConfig(raw)
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
