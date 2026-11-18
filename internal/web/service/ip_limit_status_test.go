package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
)

func TestNativeSingBoxIPLimitWorksWithoutFail2ban(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	t.Setenv("XUI_ENABLE_FAIL2BAN", "false")
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })

	if err := (&SettingService{}).SetCoreType(CoreTypeSingBox); err != nil {
		t.Fatalf("select sing-box: %v", err)
	}
	status := (&ServerService{}).GetFail2banStatus()
	if !status.Usable || status.Method != "native" {
		t.Fatalf("sing-box must support IP limits without Fail2ban: %+v", status)
	}
	if status.Installed {
		t.Fatalf("Fail2ban was disabled, but status marked it installed: %+v", status)
	}

	if err := (&SettingService{}).SetCoreType(CoreTypeXray); err != nil {
		t.Fatalf("select xray: %v", err)
	}
	status = (&ServerService{}).GetFail2banStatus()
	if status.Usable || status.Method != "unavailable" {
		t.Fatalf("legacy core must not advertise native IP-limit support: %+v", status)
	}
}

func TestNativeUpstreamIPLimitWithoutFail2ban(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	t.Setenv("XUI_ENABLE_FAIL2BAN", "false")
	if err := database.InitDB(config.GetDBPath()); err != nil {
		t.Fatalf("init db: %v", err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	if err := (&SettingService{}).SetCoreType(CoreTypeSingBoxUpstream); err != nil {
		t.Fatalf("select upstream sing-box: %v", err)
	}
	status := (&ServerService{}).GetFail2banStatus()
	if !status.Usable || status.Method != "native" {
		t.Fatalf("upstream sing-box must support native disconnection: %+v", status)
	}
}
