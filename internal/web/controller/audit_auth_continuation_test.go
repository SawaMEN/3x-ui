package controller

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/crypto"
)

func seedAuditAuthToken(t *testing.T) string {
	t.Helper()
	const token = "audit-CaseSensitive-token"
	if err := database.GetDB().Create(&model.ApiToken{
		Name: "audit-auth", Token: crypto.HashTokenSHA256(token), Enabled: true, Scope: model.ApiScopeAdmin,
	}).Error; err != nil {
		t.Fatal(err)
	}
	return token
}

func TestAuditAuthBearerSyntax(t *testing.T) {
	engine, _ := newAPIAuthTestEngine(t)
	token := seedAuditAuthToken(t)
	for _, tc := range []struct {
		header string
		status int
	}{
		{"Bearer " + token, http.StatusOK},
		{"bearer " + token, http.StatusOK},
		{"BEARER " + token, http.StatusOK},
		{"bEaReR   " + token, http.StatusOK},
		{"bearer wrong-token", http.StatusUnauthorized},
		{"BEARER", http.StatusUnauthorized},
		{"Bearer ", http.StatusUnauthorized},
		{"bearer audit-casesensitive-token", http.StatusUnauthorized},
		{"", http.StatusNotFound},
		{"Basic ignored", http.StatusNotFound},
	} {
		t.Run(tc.header, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/panel/api/ping", nil)
			req.Header.Set("Authorization", tc.header)
			w := httptest.NewRecorder()
			engine.ServeHTTP(w, req)
			if w.Code != tc.status {
				t.Fatalf("status = %d, want %d; body=%s", w.Code, tc.status, w.Body.String())
			}
		})
	}
}

func TestAuditAuthRequiresPrincipal(t *testing.T) {
	for _, auth := range []string{"bearer", "mtls"} {
		for _, failure := range []string{"missing-user", "query-error"} {
			t.Run(auth+"/"+failure, func(t *testing.T) {
				engine, _ := newAPIAuthTestEngine(t)
				token := seedAuditAuthToken(t)
				db := database.GetDB()
				if failure == "missing-user" {
					if err := db.Where("1 = 1").Delete(&model.User{}).Error; err != nil {
						t.Fatal(err)
					}
				} else {
					if err := db.Callback().Query().Before("gorm:query").Register("audit:user-read-failure", func(tx *gorm.DB) {
						if tx.Statement.Table == "users" {
							tx.AddError(errors.New("injected principal query failure"))
						}
					}); err != nil {
						t.Fatal(err)
					}
				}
				req := httptest.NewRequest(http.MethodGet, "/panel/api/server/status", nil)
				if auth == "bearer" {
					req.Header.Set("Authorization", "Bearer "+token)
				} else {
					req.TLS = &tls.ConnectionState{VerifiedChains: [][]*x509.Certificate{{{}}}}
				}
				w := httptest.NewRecorder()
				engine.ServeHTTP(w, req)
				if w.Code != http.StatusServiceUnavailable {
					t.Fatalf("status = %d, want 503 without reaching handler; body=%s", w.Code, w.Body.String())
				}
			})
		}
	}
}
