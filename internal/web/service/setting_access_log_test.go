package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultSettingsWithoutGeneratedXrayConfig(t *testing.T) {
	setupSettingTestDB(t)
	binDir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", binDir)
	s := &SettingService{}
	if err := s.setString("xrayTemplateConfig", `{"log":{"access":"/tmp/xray-access.log"}}`); err != nil {
		t.Fatal(err)
	}
	settings, err := s.GetDefaultSettings("panel.example")
	if err != nil {
		t.Fatalf("settings before core startup: %v", err)
	}
	if !settings.(map[string]any)["accessLogEnable"].(bool) {
		t.Fatal("stored Xray access log setting was lost")
	}
	if err := os.WriteFile(filepath.Join(binDir, "config.json"), []byte(`{"log":{"access":"none"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetAccessLogEnable()
	if err != nil || got {
		t.Fatalf("generated Xray config must take precedence: %t, %v", got, err)
	}
}
