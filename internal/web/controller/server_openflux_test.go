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

func TestOpenFluxProviderProbeRejectsRemoteNodeAndUnknownTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	controller := &ServerController{}
	controller.initExternalVPNRouter(engine.Group("/server"))
	for _, tc := range []struct {
		path string
		body string
	}{
		{"/server/openflux/check-transport", `{"type":"http://127.0.0.1"}`},
		{"/server/openflux/check-transport?nodeId=7", `{"type":"vyandex"}`},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if !strings.Contains(w.Body.String(), `"success":false`) {
			t.Fatalf("untrusted transport probe accepted: %s", w.Body.String())
		}
	}
}

func TestOpenFluxEndToEndRequiresSavedInboundAndExplicitConfirmation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	controller := &ServerController{}
	controller.initExternalVPNRouter(engine.Group("/server"))
	for _, tc := range []struct{ body, want string }{
		{`{"inboundId":0,"type":"direct","confirmInterruption":true}`, "saved OpenFlux"},
		{`{"inboundId":17,"type":"http://127.0.0.1","confirmInterruption":true}`, "known transport"},
		{`{"inboundId":17,"type":"yandex","confirmInterruption":false}`, "temporarily interrupt"},
		{`{"inboundId":17,"type":"yandex","confirmInterruption":true}`, "authentication required"},
	} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/server/openflux/diagnose-transport", strings.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		engine.ServeHTTP(w, req)
		if !strings.Contains(w.Body.String(), `"success":false`) || !strings.Contains(w.Body.String(), tc.want) {
			t.Fatalf("unsafe diagnostic was accepted: %s", w.Body.String())
		}
	}
}
