package controller

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestProxyPresetRoutesRegistered(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/panel/api/hosts")
	NewHostController(group)

	routes := map[string]bool{}
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{
		"GET /panel/api/hosts/presets/list",
		"GET /panel/api/hosts/presets/get/:id",
		"POST /panel/api/hosts/presets/save",
		"POST /panel/api/hosts/presets/update/:id",
		"POST /panel/api/hosts/presets/del/:id",
		"POST /panel/api/hosts/presets/preview",
		"GET /panel/api/hosts/presets/assignments",
		"GET /panel/api/hosts/presets/assignment/:groupId",
		"POST /panel/api/hosts/presets/assign/:groupId",
		"POST /panel/api/hosts/presets/assign/bulk",
		"POST /panel/api/hosts/presets/unassign/:groupId",
		"GET /panel/api/hosts/presets/bundle",
		"POST /panel/api/hosts/presets/bundle/import",
	} {
		if !routes[want] {
			t.Fatalf("missing route %q; routes=%v", want, engine.Routes())
		}
	}
}
