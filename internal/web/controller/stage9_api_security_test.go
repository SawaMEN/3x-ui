package controller

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/crypto"
)

func TestStage9APIAuthScopeMatrix(t *testing.T) {
	engine, _ := newAPIAuthTestEngine(t)
	for _, scope := range []string{model.ApiScopeMonitor, model.ApiScopeNodeSync, model.ApiScopeNodeAdmin, model.ApiScopeAdmin} {
		token := "stage9-" + scope + "-secret"
		if err := database.GetDB().Create(&model.ApiToken{
			Name: scope, Token: crypto.HashTokenSHA256(token), Enabled: true, Scope: scope,
		}).Error; err != nil {
			t.Fatalf("seed %s: %v", scope, err)
		}
	}
	cases := []struct {
		scope, method, path string
		want                int
	}{
		{model.ApiScopeMonitor, http.MethodGet, "/panel/api/server/status", http.StatusOK},
		{model.ApiScopeMonitor, http.MethodGet, "/panel/api/ping", http.StatusForbidden},
		{model.ApiScopeMonitor, http.MethodPost, "/panel/api/server/updatePanel", http.StatusForbidden},
		{model.ApiScopeNodeSync, http.MethodGet, "/panel/api/server/status", http.StatusOK},
		{model.ApiScopeNodeSync, http.MethodPost, "/panel/api/clients/test@example.org/detach", http.StatusOK},
		{model.ApiScopeNodeSync, http.MethodPost, "/panel/api/server/updatePanel", http.StatusForbidden},
		{model.ApiScopeNodeAdmin, http.MethodPost, "/panel/api/server/updatePanel", http.StatusOK},
		{model.ApiScopeNodeAdmin, http.MethodGet, "/panel/api/ping", http.StatusForbidden},
		{model.ApiScopeAdmin, http.MethodGet, "/panel/api/ping", http.StatusOK},
	}
	for _, tt := range cases {
		t.Run(tt.scope+" "+tt.method+" "+tt.path, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			req.Header.Set("Authorization", "Bearer stage9-"+tt.scope+"-secret")
			rec := httptest.NewRecorder()
			engine.ServeHTTP(rec, req)
			if rec.Code != tt.want {
				t.Fatalf("%s %s with scope %s: got %d, want %d", tt.method, tt.path, tt.scope, rec.Code, tt.want)
			}
		})
	}
}

func TestStage9UnverifiedProxyCertificateHeadersCannotAuthorizeAPI(t *testing.T) {
	engine, _ := newAPIAuthTestEngine(t)
	req := httptest.NewRequest(http.MethodPost, "/panel/api/server/updatePanel", nil)
	req.Header.Set("X-SSL-Client-Verify", "SUCCESS")
	req.Header.Set("X-Forwarded-Client-Cert", "By=malicious;Subject=CN=admin")
	req.Header.Set("X-Forwarded-For", "127.0.0.1")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unverified HTTP certificate headers elevated API privileges: status=%d body=%s", w.Code, w.Body.String())
	}
}
