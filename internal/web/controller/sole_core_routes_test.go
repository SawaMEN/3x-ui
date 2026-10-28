package controller

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCoreManagementRoutes(t *testing.T) {
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
management := map[string]bool{"GET /panel/api/setting/cores": false, "POST /panel/api/setting/cores/:core/install": false, "POST /panel/api/setting/cores/:core/select": false}
	for _, route := range engine.Routes() {
 if _, ok := management[route.Method+" "+route.Path]; ok { management[route.Method+" "+route.Path] = true }
		if removed[route.Path] {
			t.Errorf("standalone management still registered: %s %s", route.Method, route.Path)
		}
		if route.Method == http.MethodPost && route.Path == "/panel/api/server/restartCoreService" {
			nativeRestart = true
		}
	}
	for route, registered := range management {
		if !registered {
			t.Errorf("core route missing: %s", route)
		}
	}
	if !nativeRestart {
		t.Fatal("sole-core restart route is missing")
	}
}
