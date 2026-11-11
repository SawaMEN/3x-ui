package controller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/crypto"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

type auditRelayRuntime struct {
	runtime.Runtime
	runtime.SessionRuntime
	calls int
}

func (r *auditRelayRuntime) DisconnectUsersSessions(context.Context, string, []string) (int, error) {
	r.calls++
	return 1, nil
}

func (r *auditRelayRuntime) DomainActivity(context.Context) (json.RawMessage, error) {
	r.calls++
	return json.RawMessage(`{"enabled":true,"entries":[]}`), nil
}

func TestAuditNodeCredentialsCannotRelayDomainActivity(t *testing.T) {
	engine, api := newAPIAuthTestEngine(t)
	server := &ServerController{}
	group := engine.Group("/panel/api", api.checkAPIAuth, api.enforceTokenScope)
	group.GET("/server/domainActivity", server.getDomainActivity)
	group.POST("/server/singbox/sessions/disconnect-users", server.disconnectSingBoxUsersSessions)
	manager := runtime.NewManager(runtime.LocalDeps{})
	target := &auditRelayRuntime{}
	manager.SetRuntimeOverride(88, target)
	previous := runtime.GetManager()
	runtime.SetManager(manager)
	t.Cleanup(func() { runtime.SetManager(previous) })
	for _, scope := range []string{model.ApiScopeNodeSync, model.ApiScopeNodeAdmin, model.ApiScopeAdmin} {
		if err := database.GetDB().Create(&model.ApiToken{Name: scope, Token: crypto.HashTokenSHA256(scope), Scope: scope, Enabled: true}).Error; err != nil {
			t.Fatal(err)
		}
		for _, route := range []struct{ method, path, body string }{
			{http.MethodGet, "/panel/api/server/domainActivity?nodeId=88", ""},
			{http.MethodPost, "/panel/api/server/singbox/sessions/disconnect-users", `{"nodeId":88,"users":["client"]}`},
		} {
			req := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
			req.Header.Set("Authorization", "Bearer "+scope)
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			before := target.calls
			engine.ServeHTTP(response, req)
			var result struct {
				Success bool `json:"success"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			allowed := scope == model.ApiScopeAdmin
			if result.Success != allowed || (target.calls > before) != allowed {
				t.Errorf("scope=%s path=%s success=%v target calls=%d; only admin may relay", scope, route.path, result.Success, target.calls-before)
			}
		}
	}
}
