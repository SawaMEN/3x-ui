package domainfront

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/web/network"
)

func TestInheritedCertificateRenewsWithoutSecondIssuer(t *testing.T) {
	cert, key, _ := testCertificate(t, "shared.example.com", 45)
	cfg := DefaultConfig()
	cfg.Enabled, cfg.Listen, cfg.HTTPPort, cfg.HTTPSPort = true, "127.0.0.1", unusedPort(t), unusedPort(t)
	cfg.Domains = []DomainFrontDomain{{Name: "shared.example.com", Mode: "sub_link_only", Enabled: true, TLS: "auto"}}
	backend := Backend{SubscriptionTarget: "http://127.0.0.1:1", Certificates: map[string]CertificateFiles{"shared.example.com": {CertFile: cert, KeyFile: key, Source: "panel", Renewal: "acme.sh", AutoRenew: true}}}
	manager := NewManager(t.TempDir())
	defer manager.Stop()
	if err := manager.Apply(context.Background(), cfg, backend); err != nil {
		t.Fatal(err)
	}
	if manager.runtime.external["shared.example.com"] == nil {
		t.Fatal("existing certificate was not reused")
	}
	before, _ := manager.runtime.tlsConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "shared.example.com"})
	nextCert, nextKey, _ := testCertificate(t, "shared.example.com", 90)
	raw, _ := os.ReadFile(nextCert)
	if err := os.WriteFile(cert, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	partial, _ := manager.runtime.tlsConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "shared.example.com"})
	if string(partial.Certificate[0]) != string(before.Certificate[0]) {
		t.Fatal("served mismatched renewed pair")
	}
	raw, _ = os.ReadFile(nextKey)
	if err := os.WriteFile(key, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	after, err := manager.runtime.tlsConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "shared.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(after.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(after.Certificate[0]) == string(before.Certificate[0]) || leaf.NotAfter.Equal(before.Leaf.NotAfter) {
		t.Fatal("renewed certificate was not picked up")
	}
	status := manager.Status(cfg).Certificates[0]
	if status.DaysLeft != 90 || status.Source != "panel" || status.Renewal != "acme.sh" || !status.AutoRenew {
		t.Fatalf("renewal status = %#v", status)
	}
}

func TestGatewaySharesPanelListenerWithoutProxyLoop(t *testing.T) {
	cert, key, roots := testCertificate(t, "shared.example.com", 45)
	pair, err := tls.LoadX509KeyPair(cert, key)
	if err != nil {
		t.Fatal(err)
	}
	manager := NewManager(t.TempDir())
	defer manager.Stop()
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "existing:"+r.Host) })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	sharedURL := fmt.Sprintf("https://127.0.0.1:%d", port)
	shared := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !manager.ServeShared(w, r, "127.0.0.1", port, true) {
			fallback.ServeHTTP(w, r)
		}
	})}
	shared.Protocols = new(http.Protocols)
	shared.Protocols.SetHTTP1(true)
	shared.Protocols.SetHTTP2(true)
	tlsConfig := &tls.Config{NextProtos: []string{"h2", "http/1.1", "acme-tls/1"}, GetCertificate: func(h *tls.ClientHelloInfo) (*tls.Certificate, error) {
		c, err := manager.SharedCertificate(h, "127.0.0.1", port)
		if c != nil || err != nil {
			return c, err
		}
		return &pair, nil
	}}
	go func() { _ = network.ServeHTTP(shared, tls.NewListener(listener, tlsConfig), "Shared test server") }()
	defer shared.Close()
	cfg := DefaultConfig()
	cfg.Enabled, cfg.Listen, cfg.HTTPPort, cfg.HTTPSPort = true, "127.0.0.1", unusedPort(t), port
	cfg.Domains = []DomainFrontDomain{{Name: "shared.example.com", Mode: "direct", Enabled: true, Panel: true, TLS: "auto"}}
	backend := Backend{SharedHTTPS: true, PanelTarget: sharedURL, PanelHost: "private.example.com", PanelPath: "/secret/", PanelHandler: fallback, SubscriptionTarget: "http://127.0.0.1:1", Certificates: map[string]CertificateFiles{"shared.example.com": {CertFile: cert, KeyFile: key, Source: "panel"}}}
	if err := manager.Apply(context.Background(), cfg, backend); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: &http.Transport{ForceAttemptHTTP2: true, TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "shared.example.com"}}}
	defer client.CloseIdleConnections()
	req, _ := http.NewRequest(http.MethodGet, sharedURL+"/secret/panel/", nil)
	req.Host = "shared.example.com"
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || response.ProtoMajor != 2 || string(raw) != "existing:private.example.com" {
		t.Fatalf("shared response = %d %q", response.StatusCode, raw)
	}
	backend.PanelHandler = http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "updated-route") })
	if err := manager.RefreshRoutes(backend); err != nil {
		t.Fatal(err)
	}
	response, err = client.Do(req.Clone(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(raw) != "updated-route" {
		t.Fatalf("route update did not reach the shared listener: %q", raw)
	}
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	broken := cfg
	broken.HTTPPort = occupied.Addr().(*net.TCPAddr).Port
	if err := manager.Apply(context.Background(), broken, backend); err == nil {
		t.Fatal("occupied HTTP port unexpectedly applied")
	}
	response, err = client.Do(req.Clone(context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = io.ReadAll(response.Body)
	_ = response.Body.Close()
	if string(raw) != "updated-route" {
		t.Fatalf("rollback lost refreshed routes: %q", raw)
	}
}

func TestExistingACMEChallengeAndLiveRedirect(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".well-known", "acme-challenge")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "token_123"), []byte("validation"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := "https://first.example.com/"
	cfg := DefaultConfig()
	cfg.Domains = []DomainFrontDomain{{Name: "shared.example.com", Mode: "sub_link_only", Enabled: true}}
	r := &frontRuntime{cfg: cfg, backend: Backend{SubscriptionTarget: "http://127.0.0.1:1", ChallengeRoots: map[string]string{"shared.example.com": root}, GetRedirect: func() (string, error) { return target, nil }}}
	w := httptest.NewRecorder()
	r.externalChallenge(w, httptest.NewRequest(http.MethodGet, "http://shared.example.com/.well-known/acme-challenge/token_123", nil))
	if w.Code != 200 || w.Body.String() != "validation" {
		t.Fatalf("challenge = %d %q", w.Code, w.Body.String())
	}
	handler, err := r.handler()
	if err != nil {
		t.Fatal(err)
	}
	target = "https://updated.example.com/"
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://shared.example.com/", nil))
	if w.Code != 302 || w.Header().Get("Location") != target {
		t.Fatalf("live redirect = %d %s", w.Code, w.Header().Get("Location"))
	}
}
