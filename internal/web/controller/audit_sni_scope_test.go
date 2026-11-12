package controller

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/crypto"
	webruntime "github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

func TestSNIJobsAuthenticatedNodeCredentialsCannotRelay(t *testing.T) {
	for _, scope := range []string{model.ApiScopeNodeSync, model.ApiScopeNodeAdmin, model.ApiScopeAdmin} {
		t.Run(scope, func(t *testing.T) {
			engine, api := newAPIAuthTestEngine(t)
			group := engine.Group("/panel/api/server")
			group.Use(api.checkAPIAuth, api.enforceTokenScope)
			(&ServerController{}).initSNIJobRouter(group)
			token := "synthetic-sni-" + scope
			if err := database.GetDB().Create(&model.ApiToken{Name: scope, Token: crypto.HashTokenSHA256(token), Enabled: true, Scope: scope}).Error; err != nil {
				t.Fatal(err)
			}
			manager := webruntime.NewManager(webruntime.LocalDeps{})
			previous := webruntime.GetManager()
			webruntime.SetManager(manager)
			defer webruntime.SetManager(previous)
			stub := &sniRemoteStub{}
			manager.SetRuntimeOverride(7, stub)
			for _, endpoint := range []struct{ method, path, body string }{
				{http.MethodPost, "/jobs", `{"nodeId":7,"mode":"combined"}`},
				{http.MethodGet, "/jobs/remote-job?nodeId=7", ""},
				{http.MethodDelete, "/jobs/remote-job?nodeId=7", ""},
				{http.MethodGet, "/history?nodeId=7", ""},
			} {
				before := stub.calls
				req := httptest.NewRequest(endpoint.method, "/panel/api/server/sni"+endpoint.path, strings.NewReader(endpoint.body))
				req.Header.Set("Authorization", "Bearer "+token)
				req.Header.Set("Content-Type", "application/json")
				env := doGeodataReq(t, engine, req)
				allowed := scope == model.ApiScopeAdmin
				if env.Success != allowed || (stub.calls != before) != allowed {
					t.Fatalf("%s %s scope=%s: success=%v, remote calls=%d", endpoint.method, endpoint.path, scope, env.Success, stub.calls-before)
				}
			}
		})
	}
}

func TestSNINodeCredentialsKeepLocalTargets(t *testing.T) {
	for _, scope := range []string{model.ApiScopeNodeSync, model.ApiScopeNodeAdmin} {
		for _, id := range []*int{nil, new(int)} {
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Set("api_token_scope", scope)
			if target, err := normalizeSNINodeTarget(c, id); err != nil || target != nil {
				t.Fatalf("local scope=%s target=%v error=%v", scope, target, err)
			}
		}
	}
}
