package sub

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestSubscriptionDomainAliasesDoNotAllowPanelHost(t *testing.T) {
	initSubDB(t)
	for key, value := range map[string]string{"subPublicURL": "https://cdn.example.com", "subAllowedDomains": "extra.example.com"} {
		if err := database.GetDB().Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer()
	router := gin.New()
	router.Use(s.subscriptionDomainValidator("origin.example.com"))
	router.NoRoute(func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, tc := range []struct {
		host   string
		status int
	}{
		{"origin.example.com", 204},
		{"cdn.example.com", 204},
		{"extra.example.com", 204},
		{"CDN.EXAMPLE.COM.:443", 204},
		{"panel.example.com", 403},
		{"attacker.example.com", 403},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://"+tc.host+"/sub/token", nil))
		if response.Code != tc.status {
			t.Fatalf("%s status = %d, want %d", tc.host, response.Code, tc.status)
		}
	}
}

func TestRestrictedSubscriptionRejectsListenerHost(t *testing.T) {
	initSubDB(t)
	for key, value := range map[string]string{
		"subPublicURL":      "https://first.example.com:8443",
		"subAllowedDomains": "second.example.com", "subRestrictDomains": "true",
	} {
		if err := database.GetDB().Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
			t.Fatal(err)
		}
	}
	s := NewServer()
	router := gin.New()
	router.Use(s.subscriptionDomainValidator("origin.example.com"))
	router.NoRoute(func(c *gin.Context) { c.Status(http.StatusNoContent) })
	for _, tc := range []struct {
		host   string
		status int
	}{
		{"first.example.com:8443", 204},
		{"second.example.com:8443", 204},
		{"origin.example.com", 403},
		{"panel.example.com", 403},
	} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "https://"+tc.host+"/sub/token", nil))
		if response.Code != tc.status {
			t.Fatalf("%s = %d, want %d", tc.host, response.Code, tc.status)
		}
	}
}

func TestManagedSubscriptionPageServesTLSOnConfiguredPort(t *testing.T) {
	const name = "subscriptions.example.com"
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	logger.InitLogger(logging.ERROR)
	initSubDB(t)
	seedInfoEndpointSub(t, "managed-sub", "managed@example.com")
	oldFS := distFS
	SetDistFS(testDistFS)
	t.Cleanup(func() { SetDistFS(oldFS) })

	cert, key := anyTLSCertificate(t, name)
	subPort := anyTLSFreePort(t)
	cfg := domainfront.DefaultConfig()
	cfg.Enabled, cfg.Listen, cfg.HTTPPort, cfg.HTTPSPort = true, "127.0.0.1", anyTLSFreePort(t), anyTLSFreePort(t)
	cfg.PublicPort, cfg.SubscriptionDomain = cfg.HTTPSPort, name
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: name, Enabled: true, Mode: "sub_link_only", TLS: "manual", CertFile: cert, KeyFile: key}}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"domainFrontConfig": string(raw), "subDomain": name, "subPort": strconv.Itoa(subPort), "subPath": "/sub/",
	} {
		if err := database.GetDB().Where(model.Setting{Key: key}).Assign(model.Setting{Value: value}).FirstOrCreate(&model.Setting{}).Error; err != nil {
			t.Fatal(err)
		}
	}
	server := NewServer()
	if err := server.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = server.Stop() })
	if err := service.StartDomainFront(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(service.StopDomainFront)
	pem, err := os.ReadFile(cert)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pem) {
		t.Fatal("invalid test certificate")
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: name}},
	}
	t.Cleanup(client.CloseIdleConnections)
	host := net.JoinHostPort(name, strconv.Itoa(subPort))
	const path = "/sub/managed-sub"
	for _, query := range []string{"", "?format=info"} {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://"+net.JoinHostPort("127.0.0.1", strconv.Itoa(subPort))+path+query, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = host
		req.Header.Set("Accept", "text/html")
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK || len(resp.TLS.VerifiedChains) == 0 {
			_ = resp.Body.Close()
			t.Fatalf("subscription response = %d", resp.StatusCode)
		}
		if query != "" {
			var info map[string]any
			err := json.NewDecoder(resp.Body).Decode(&info)
			if err != nil || info["subUrl"] != "https://"+host+path {
				_ = resp.Body.Close()
				t.Fatalf("subscription URL = %v, %v", info["subUrl"], err)
			}
		} else if !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
			_ = resp.Body.Close()
			t.Fatalf("subscription page content type = %q", resp.Header.Get("Content-Type"))
		}
		_ = resp.Body.Close()
	}
}
