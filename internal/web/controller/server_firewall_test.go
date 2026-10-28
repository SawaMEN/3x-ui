package controller

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/gin-gonic/gin"
)

func TestFirewallStatusInitializesAutoSyncWithoutEnablingFirewall(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "panel.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.CloseDB() })
	t.Setenv("PATH", dir) // The test must never invoke the host firewall.
	router := gin.New()
	NewFirewallController(router.Group("/panel/api/server"))
	request := httptest.NewRequest(http.MethodGet, "http://panel.example:8443/panel/api/server/firewall/status", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status: %s", response.Body)
	}
	for key, want := range map[string]string{"firewallControlInitialized": "true", "firewallSafetyPort": "8443"} {
		var setting model.Setting
		if err := database.GetDB().Where("key = ?", key).First(&setting).Error; err != nil {
			t.Fatal(err)
		}
		if setting.Value != want {
			t.Fatalf("%s = %q, want %q", key, setting.Value, want)
		}
	}
	var count int64
	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "firewallManagedEnabled").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("reading status must not enable or disable the firewall")
	}
}
