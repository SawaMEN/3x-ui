package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/session"
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
	engine.Use(sessions.Sessions("test", cookie.NewStore([]byte("openflux-test-session-secret"))))
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

func TestOpenFluxDiagnosticResultRestrictedToJobOwner(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const runID = "test-diagnostic-private-job"
	openFluxDiagnosticJobs.Lock()
	openFluxDiagnosticJobs.entries[runID] = &openFluxDiagnosticJob{
		RunID: runID, State: "success", OwnerID: 15, Until: time.Now().Add(time.Minute),
	}
	openFluxDiagnosticJobs.Unlock()
	t.Cleanup(func() {
		openFluxDiagnosticJobs.Lock()
		delete(openFluxDiagnosticJobs.entries, runID)
		openFluxDiagnosticJobs.Unlock()
	})
	for _, tc := range []struct{ owner int; permitted bool }{
		{15, true}, {16, false},
	} {
		engine := gin.New()
		engine.Use(func(c *gin.Context) {
			session.SetAPIAuthUser(c, &model.User{Id: tc.owner})
			c.Next()
		})
		controller := &ServerController{}
		controller.initExternalVPNRouter(engine.Group("/server"))
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/server/openflux/diagnose-status?runId="+runID, nil))
		body := w.Body.String()
		if !strings.Contains(body, `"success":`+map[bool]string{true:"true", false:"false"}[tc.permitted]) {
			t.Fatalf("owner=%d response=%s", tc.owner, body)
		}
		if tc.permitted && !strings.Contains(body, runID) {
			t.Fatalf("owner unable to read job: %s", body)
		}
		if !tc.permitted && strings.Contains(body, runID) {
			t.Fatalf("job ID leaked to another user: %s", body)
		}
	}
}

func TestOpenFluxTransportDiagnosticsAcceptLegacyFormAndJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("test", cookie.NewStore([]byte("openflux-format-test-secret"))))
	controller := &ServerController{}
	controller.initExternalVPNRouter(engine.Group("/server"))
	for _, tc := range []struct {
		name, path, contentType, body, want string
	}{
		{"TLS check form", "/server/openflux/check-transport", "application/x-www-form-urlencoded", "type=tyandex", "has no provider-side connectivity check"},
		{"TLS check JSON", "/server/openflux/check-transport", "application/json", `{"type":"tyandex"}`, "has no provider-side connectivity check"},
		{"E2E check form", "/server/openflux/diagnose-transport", "application/x-www-form-urlencoded", "inboundId=0&type=yandex&confirmInterruption=true", "saved OpenFlux inbound"},
		{"E2E check JSON", "/server/openflux/diagnose-transport", "application/json", `{"inboundId":0,"type":"yandex","confirmInterruption":true}`, "saved OpenFlux inbound"},
		{"link import form", "/server/externalvpn/parse-link", "application/x-www-form-urlencoded", "link=invalid", "false"},
		{"link import JSON", "/server/externalvpn/parse-link", "application/json", `{"link":"invalid"}`, "false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, tc.path, strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if !strings.Contains(w.Body.String(), tc.want) || strings.Contains(w.Body.String(), "invalid character") {
				t.Fatalf("unexpected JSON decoder failure: %s", w.Body.String())
			}
		})
	}
}
