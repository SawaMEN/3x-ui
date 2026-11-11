package controller

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/crypto"
)

func TestAuditLoginSessionFailureResponse(t *testing.T) {
	_, _ = newAPIAuthTestEngine(t)
	hash, err := crypto.HashPasswordAsBcrypt("audit-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.User{Username: "audit-login", Password: hash}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Callback().Create().Before("gorm:create").Register("audit:session-store-failure", func(tx *gorm.DB) {
		if tx.Statement.Table == "user_sessions" {
			tx.AddError(errors.New("private database failure detail"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(sessions.Sessions("3x-ui", cookie.NewStore([]byte("audit-login-cookie-secret"))))
	a := &IndexController{}
	router.POST("/login", a.login)
	form := url.Values{"username": {"audit-login"}, "password": {"audit-password"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%q, want 503 JSON", w.Code, w.Body.String())
	}
	var response struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Success {
		t.Fatalf("failure response=%q, decode error=%v", w.Body.String(), err)
	}
	if strings.Contains(w.Body.String(), "private database failure detail") {
		t.Fatal("database failure detail leaked to browser")
	}
}

func TestAuditCSRFTokenResponseDoesNotCache(t *testing.T) {
	for _, handler := range []struct {
		name  string
		serve gin.HandlerFunc
	}{
		{"public", (&IndexController{}).csrfToken},
		{"panel", (&XUIController{}).csrfToken},
	} {
		t.Run(handler.name, func(t *testing.T) {
			auditCSRFTokenResponseDoesNotCache(t, handler.serve)
		})
	}
}

func auditCSRFTokenResponseDoesNotCache(t *testing.T, handler gin.HandlerFunc) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("3x-ui", cookie.NewStore([]byte("audit-csrf-cookie-secret"))))
	router.GET("/csrf-token", handler)
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/csrf-token", nil))
	req := httptest.NewRequest(http.MethodGet, "/csrf-token", nil)
	for _, cookie := range first.Result().Cookies() {
		req.AddCookie(cookie)
	}
	second := httptest.NewRecorder()
	router.ServeHTTP(second, req)
	for _, w := range []*httptest.ResponseRecorder{first, second} {
		if w.Code != http.StatusOK || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("status=%d Cache-Control=%q, want 200/no-store", w.Code, w.Header().Get("Cache-Control"))
		}
	}
	if first.Body.String() != second.Body.String() {
		t.Fatal("CSRF token changed for the same cookie")
	}
}
