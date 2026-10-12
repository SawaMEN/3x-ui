package controller

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestStandaloneCoreManagementRoutesRemoved(t *testing.T) {
	engine := gin.New()
	(&ServerController{}).initRouter(engine.Group("/panel/api/server"))
	(&SettingController{}).initRouter(engine.Group("/panel/api"))
	removed := map[string]bool{
		"/panel/api/server/getXrayVersion":            true,
		"/panel/api/server/installXray/:version":      true,
		"/panel/api/setting/singbox/install":          true,
		"/panel/api/setting/singbox/install/:version": true,
		"/panel/api/setting/singbox/uninstall":        true,
		"/panel/api/setting/singbox/versions":         true,
	}
	nativeRestart := false
	for _, route := range engine.Routes() {
		if removed[route.Path] {
			t.Errorf("standalone management still registered: %s %s", route.Method, route.Path)
		}
		if route.Method == http.MethodPost && route.Path == "/panel/api/server/restartCoreService" {
			nativeRestart = true
		}
	}
	if !nativeRestart {
		t.Fatal("sole-core restart route is missing")
	}
}
