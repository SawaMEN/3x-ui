package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	webruntime "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

type openFluxNodeInstaller struct {
	webruntime.Runtime
	starts int
}

func (r *openFluxNodeInstaller) Name() string { return "test-node" }
func (r *openFluxNodeInstaller) StartOpenFluxInstall(context.Context) (json.RawMessage, error) {
	r.starts++
	return json.RawMessage(`{"runId":"node-job","state":"running"}`), nil
}
func (r *openFluxNodeInstaller) ReadOpenFluxInstall(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"runId":"node-job","state":"success"}`), nil
}
func (r *openFluxNodeInstaller) ReadOpenFluxStatus(context.Context) (json.RawMessage, error) {
	return json.RawMessage(`{"installed":true,"supported":true}`), nil
}

func TestOpenFluxInstallationTargetsSelectedNode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	manager := webruntime.NewManager(webruntime.LocalDeps{})
	node := &openFluxNodeInstaller{}
	manager.SetRuntimeOverride(7, node)
	previous := webruntime.GetManager()
	webruntime.SetManager(manager)
	t.Cleanup(func() { webruntime.SetManager(previous) })
	engine := gin.New()
	controller := &ServerController{}
	controller.initExternalVPNRouter(engine.Group("/server"))
	for _, test := range []struct{ method, path, want string }{
		{http.MethodPost, "/server/openflux/install?nodeId=7", "node-job"},
		{http.MethodGet, "/server/openflux/install?nodeId=7", "success"},
		{http.MethodGet, "/server/openflux/status?nodeId=7", "installed"},
	} {
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		if !strings.Contains(w.Body.String(), test.want) || !strings.Contains(w.Body.String(), `"success":true`) {
			t.Fatalf("%s: %s", test.path, w.Body.String())
		}
	}
	if node.starts != 1 {
		t.Fatalf("remote install starts=%d", node.starts)
	}
}

func TestOpenFluxInstallationScopeIsRestricted(t *testing.T) {
	if scopeAllows(nodeSyncScopeAllow, "/server/openflux/install", http.MethodPost) ||
		scopeAllows(nodeSyncScopeAllow, "/server/openflux/install", http.MethodGet) {
		t.Fatal("node-sync may not manage installation jobs")
	}
	if !scopeAllows(nodeAdminScopeAllow, "/server/openflux/install", http.MethodPost) ||
		!scopeAllows(nodeAdminScopeAllow, "/server/openflux/install", http.MethodGet) ||
		!scopeAllows(nodeSyncScopeAllow, "/server/openflux/status", http.MethodGet) {
		t.Fatal("component operation scopes are incomplete")
	}
}
