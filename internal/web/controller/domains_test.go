package controller

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestDomainAPIApplyPersistsConfigurationAndRejectsRoleViolations(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	if err := database.InitDB(filepath.Join(dir, "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { service.StopDomainFront(); _ = database.CloseDB() })
	router := gin.New()
	NewDomainController(router.Group("/panel/api"))
	cfg := domainfront.DefaultConfig()
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: "subscription.example.com", Mode: "sub_link_only", Enabled: true, TLS: "auto"}}
	post := func(value domainfront.DomainFrontConfig) struct {
		Success bool                          `json:"success"`
		Obj     domainfront.DomainFrontStatus `json:"obj"`
	} {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodPost, "/panel/api/domains/apply", bytes.NewReader(raw))
		req.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req)
		var result struct {
			Success bool                          `json:"success"`
			Obj     domainfront.DomainFrontStatus `json:"obj"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		return result
	}
	result := post(cfg)
	if !result.Success || result.Obj.Running || len(result.Obj.Config.Domains) != 1 || result.Obj.Config.Domains[0].Mode != "sub_link_only" {
		t.Fatalf("apply = %#v", result)
	}
	var before model.Setting
	if err := database.GetDB().Where("key = ?", "domainFrontConfig").First(&before).Error; err != nil {
		t.Fatal(err)
	}
	cfg.Domains[0].Panel = true
	if invalid := post(cfg); invalid.Success {
		t.Fatal("subscription-only panel access was accepted")
	}
	cfg.Domains[0].Mode = "direct"
	if invalid := post(cfg); invalid.Success {
		t.Fatal("direct domain panel access was accepted")
	}
	var after model.Setting
	if err := database.GetDB().Where("key = ?", "domainFrontConfig").First(&after).Error; err != nil {
		t.Fatal(err)
	}
	if before.Value != after.Value {
		t.Fatal("invalid domain configuration replaced the saved configuration")
	}
	req := httptest.NewRequest(http.MethodPost, "/panel/api/domains/check", bytes.NewBufferString(`{"name":"unconfigured.example.com"}`))
	req.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	var checked struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.Success {
		t.Fatal("DNS/TLS probe accepted an unconfigured domain")
	}
}

func TestSubscriptionTLSProbeUsesAdvertisedPort(t *testing.T) {
	const name = "subs.example.com"
	for _, test := range []struct {
		origin string
		want   int
	}{
		{"https://subs.example.com:8443", 8443},
		{"https://subs.example.com", 443},
		{"https://other.example.com:8443", 9443},
		{"http://subs.example.com:8080", 9443},
	} {
		if got := subscriptionTLSProbePort(name, test.origin, 9443); got != test.want {
			t.Errorf("port for %s = %d, want %d", test.origin, got, test.want)
		}
	}
}

func TestPublicTLSCheckDistinguishesPendingCertificateFromFailure(t *testing.T) {
	const name = "subs.example.com"
	certificate := domainfront.DomainFrontCertificate{Name: name, Source: "gateway", State: "pending"}
	status := domainfront.DomainFrontStatus{Running: true, Certificates: []domainfront.DomainFrontCertificate{certificate}}
	handshakeErr := errors.New("remote error: tls: internal error")
	result := domainfront.DomainFrontCheck{Name: name}
	setPublicTLSCheckFailure(&result, handshakeErr, status)
	if !result.Pending || result.Error != "" {
		t.Fatalf("pending certificate: %+v", result)
	}
	status.Certificates[0].State = "error"
	status.Certificates[0].Error = "ACME challenge failed"
	result = domainfront.DomainFrontCheck{Name: name}
	setPublicTLSCheckFailure(&result, handshakeErr, status)
	if result.Pending || result.Error != "ACME challenge failed" {
		t.Fatalf("failed issuance: %+v", result)
	}
}
