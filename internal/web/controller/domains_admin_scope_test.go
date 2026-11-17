package controller

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/crypto"
	"github.com/SawaMEN/3x-ui/v3/internal/util/wirecodec"
)

func TestDomainAdminScopeIncludesNodeAndDomainManagement(t *testing.T) {
	engine, api := newAPIAuthTestEngine(t)
	NewDomainController(engine.Group("/panel/api", api.checkAPIAuth, api.enforceTokenScope))
	for _, scope := range []string{model.ApiScopeAdmin, model.ApiScopeNodeAdmin, model.ApiScopeNodeSync, model.ApiScopeMonitor} {
		if err := database.GetDB().Create(&model.ApiToken{Name: scope, Token: crypto.HashTokenSHA256(scope), Scope: scope, Enabled: true}).Error; err != nil {
			t.Fatal(err)
		}
	}
	routes := []struct {
		method, path  string
		nodeOperation bool
	}{
		{http.MethodGet, "/domains/cover-sites/local?certificates=1", true},
		{http.MethodPost, "/domains/cover-sites/template", true},
		{http.MethodPost, "/domains/certificates/export-cover", true},
		{http.MethodPost, "/domains/certificates/resolve-local", true},
		{http.MethodPost, "/domains/certificates/install", true},
		{http.MethodPost, "/domains/apply", false},
		{http.MethodPost, "/domains/dns/credential", false},
		{http.MethodPost, "/domains/certificates/renew", false},
		{http.MethodPost, "/domains/certificates/fleet", false},
		{http.MethodPost, "/domains/certificates/prepare", false},
		{http.MethodPost, "/domains/certificates/panel-domain", false},
	}
	for _, scope := range []string{model.ApiScopeAdmin, model.ApiScopeNodeAdmin, model.ApiScopeNodeSync, model.ApiScopeMonitor} {
		for _, mtls := range []bool{false, true} {
			for _, push := range []bool{false, true} {
				for _, route := range routes {
					t.Run(scope+route.method+route.path+boolLabel(mtls)+boolLabel(push), func(t *testing.T) {
						// Malformed JSON proves the authorization gate was passed without
						// changing listeners, issuing certificates or persisting credentials.
						req := httptest.NewRequest(route.method, "/panel/api"+route.path, strings.NewReader("{"))
						req.Header.Set("Content-Type", "application/json")
						req.Header.Set("Authorization", "Bearer "+scope)
						if push {
							req.Header.Set(wirecodec.MasterPushHeader, "1")
						}
						if mtls {
							req.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{}}}}
						}
						response := httptest.NewRecorder()
						engine.ServeHTTP(response, req)
						want := http.StatusForbidden
						if scope == model.ApiScopeAdmin || (route.nodeOperation && push && (mtls || scope == model.ApiScopeNodeSync || scope == model.ApiScopeNodeAdmin)) {
							want = http.StatusOK
						}
						if response.Code != want {
							t.Fatalf("status=%d, want %d", response.Code, want)
						}
					})
				}
			}
		}
	}
}

func boolLabel(value bool) string {
	if value {
		return "/true"
	}
	return "/false"
}

func TestDomainAdminTokenMustBeValid(t *testing.T) {
	engine, api := newAPIAuthTestEngine(t)
	NewDomainController(engine.Group("/panel/api", api.checkAPIAuth, api.enforceTokenScope))
	for _, token := range []model.ApiToken{
		{Name: "disabled", Token: crypto.HashTokenSHA256("disabled"), Scope: model.ApiScopeAdmin, Enabled: false},
		{Name: "expired", Token: crypto.HashTokenSHA256("expired"), Scope: model.ApiScopeAdmin, Enabled: true, ExpiresAt: time.Now().Add(-time.Minute).UnixMilli()},
	} {
		if err := database.GetDB().Create(&token).Error; err != nil {
			t.Fatal(err)
		}
		if token.Name == "disabled" {
			if err := database.GetDB().Model(&token).Update("enabled", false).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, token := range []string{"disabled", "expired", "unknown"} {
		for _, mtls := range []bool{false, true} {
			req := httptest.NewRequest(http.MethodPost, "/panel/api/domains/certificates/fleet", strings.NewReader("{"))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+token)
			if mtls {
				req.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{}}}}
			}
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			want := http.StatusUnauthorized
			if mtls {
				want = http.StatusForbidden
			}
			if response.Code != want {
				t.Fatalf("%s mtls=%v: status=%d, want %d", token, mtls, response.Code, want)
			}
		}
	}
}
