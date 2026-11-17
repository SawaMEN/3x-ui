package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/crypto"
)

func TestAmneziaWGMetricsRequiresAuthenticationAndAllowsMonitor(t *testing.T) {
	engine, auth := newAPIAuthTestEngine(t)
	server := &ServerController{}
	api := engine.Group("/panel/api", auth.checkAPIAuth, auth.enforceTokenScope)
	api.GET("/server/amneziawgMetrics", server.getAmneziaWGMetrics)
	const token = "metrics-monitor-token"
	if err := database.GetDB().Create(&model.ApiToken{Name: "metrics", Token: crypto.HashTokenSHA256(token), Enabled: true, Scope: model.ApiScopeMonitor}).Error; err != nil {
		t.Fatal(err)
	}
	for _, authorized := range []bool{false, true} {
		req := httptest.NewRequest(http.MethodGet, "/panel/api/server/amneziawgMetrics", nil)
		if authorized {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		if !authorized {
			if response.Code != http.StatusNotFound {
				t.Fatalf("anonymous metrics status %d", response.Code)
			}
			continue
		}
		if response.Code != http.StatusOK {
			t.Fatalf("monitor metrics status %d", response.Code)
		}
		if !strings.HasPrefix(response.Header().Get("Content-Type"), "text/plain;") || response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("incorrect scrape response headers")
		}
		if !strings.Contains(response.Body.String(), "awg_inbounds_configured 0\n") {
			t.Fatal("missing Prometheus metric")
		}
	}
}
