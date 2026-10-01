package sub

import (
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterAutoSubscriptionRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	controller := &SUBController{subPath: "/sub/"}

	registerAutoSubscriptionRoute(engine.Group("/"), controller)

	routes := make(map[string]bool)
	for _, route := range engine.Routes() {
		routes[route.Method+" "+route.Path] = true
	}
	for _, want := range []string{"GET /sub/:subid/auto", "HEAD /sub/:subid/auto"} {
		if !routes[want] {
			t.Fatalf("missing route %q; routes=%v", want, engine.Routes())
		}
	}
}
