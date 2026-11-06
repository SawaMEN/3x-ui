package domainfront

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"
)

func TestDomainRolesRouteOnlyTheirPermittedPaths(t *testing.T) {
	sub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		_, _ = fmt.Fprintf(w, "subscription:%s:%s?%s", r.Host, r.URL.Path, r.URL.RawQuery)
	}))
	defer sub.Close()
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprintf(w, "panel:%s", r.Host) }))
	defer panel.Close()
	vpn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "vpn") }))
	defer vpn.Close()
	relay := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = fmt.Fprintf(w, "relay:%s", r.URL.RequestURI()) }))
	defer relay.Close()
	oldTransport := http.DefaultTransport
	http.DefaultTransport = relay.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	cfg := DefaultConfig()
	cfg.Domains = []DomainFrontDomain{
		{Name: "direct.example.com", Mode: "direct", Enabled: true, Panel: true},
		{Name: "cdn.example.com", Mode: "cdn", Enabled: true},
		{Name: "only.example.com", Mode: "sub_link_only", Enabled: true},
		{Name: "relay.example.com", Mode: "relay", Enabled: true, Origin: relay.URL},
		{Name: "off.example.com", Mode: "direct", Enabled: false},
	}
	backend := Backend{SubscriptionTarget: sub.URL, PanelTarget: panel.URL, PanelHost: "private.example.com", PanelPath: "/secret/", SubscriptionPaths: []string{"/sub/", "/json/", "/clash/", "/assets/"}, Routes: map[string][]Route{}, RedirectURL: "https://destination.example.com/"}
	for _, d := range cfg.Domains {
		backend.Routes[d.Name] = []Route{{Path: "/vpn", Target: vpn.URL}}
	}
	r := &frontRuntime{cfg: cfg, backend: backend}
	handler, err := r.handler()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		host, path string
		status     int
		body       string
	}{
		{"direct.example.com", "/sub/token?format=info", 200, "subscription:direct.example.com:/sub/token?format=info"},
		{"cdn.example.com", "/json/token", 200, "subscription:cdn.example.com:/json/token?"},
		{"only.example.com", "/assets/app.js", 200, "subscription:only.example.com:/assets/app.js?"},
		{"only.example.com", "/secret/panel/", 404, "404 page not found\n"},
		{"only.example.com", "/vpn", 404, "404 page not found\n"},
		{"direct.example.com", "/secret/panel/", 200, "panel:private.example.com"},
		{"direct.example.com", "/vpn/session", 200, "vpn"},
		{"relay.example.com", "/sub/token?format=sing-box", 200, "relay:/sub/token?format=sing-box"},
		{"cdn.example.com", "/secret/panel/", 404, "404 page not found\n"},
		{"off.example.com", "/sub/token", 404, "404 page not found\n"},
		{"unknown.example.com", "/sub/token", 404, "404 page not found\n"},
	} {
		t.Run(tc.host+tc.path, func(t *testing.T) {
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://"+tc.host+tc.path, nil))
			if w.Code != tc.status || w.Body.String() != tc.body {
				t.Fatalf("response = %d %q, want %d %q", w.Code, w.Body.String(), tc.status, tc.body)
			}
			if tc.status == 200 && strings.HasPrefix(tc.path, "/sub/") {
				if got := w.Header().Get("Cache-Control"); got != "private, no-store, max-age=0" {
					t.Fatalf("Cache-Control = %q", got)
				}
			}
		})
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "https://only.example.com/?private=secret", nil))
	if w.Code != 302 || w.Header().Get("Location") != "https://destination.example.com/" {
		t.Fatalf("redirect = %d %q", w.Code, w.Header().Get("Location"))
	}
}

func testCertificate(t *testing.T, domain string, days int) (string, string, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName: domain}, DNSNames: []string{domain}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Duration(days) * 24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certFile, certPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(certPEM) {
		t.Fatal("invalid test certificate")
	}
	return certFile, keyFile, roots
}

func unusedPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func TestManagedTLSApplyRollbackAndExpiry(t *testing.T) {
	cert, key, roots := testCertificate(t, "only.example.com", 45)
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Listen = "127.0.0.1"
	cfg.HTTPPort = unusedPort(t)
	cfg.HTTPSPort = unusedPort(t)
	cfg.PublicPort = cfg.HTTPSPort
	cfg.Domains = []DomainFrontDomain{{Name: "only.example.com", Mode: "sub_link_only", Enabled: true, TLS: "manual", CertFile: cert, KeyFile: key}}
	manager := NewManager(t.TempDir())
	t.Cleanup(manager.Stop)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok") }))
	defer upstream.Close()
	backend := Backend{SubscriptionTarget: upstream.URL, SubscriptionPaths: []string{"/sub/"}}
	if err := manager.Apply(context.Background(), cfg, backend); err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "only.example.com", MinVersion: tls.VersionTLS12}, ForceAttemptHTTP2: true}
	t.Cleanup(transport.CloseIdleConnections)
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	get := func(path string) string {
		t.Helper()
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d%s", cfg.HTTPSPort, path), nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "only.example.com"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d: %s", resp.StatusCode, body)
		}
		if resp.ProtoMajor != 2 {
			t.Fatalf("TLS gateway protocol = %s, want HTTP/2", resp.Proto)
		}
		return string(body)
	}
	if get("/sub/token") != "ok" {
		t.Fatal("TLS proxy response changed")
	}
	status := manager.Status(cfg)
	if !status.Running || status.Certificates[0].DaysLeft != 45 || status.Certificates[0].AutoRenew {
		t.Fatalf("status = %#v", status)
	}
	original, err := os.ReadFile(cert)
	if err != nil {
		t.Fatal(err)
	}
	newCert, _, _ := testCertificate(t, "only.example.com", 2)
	data, _ := os.ReadFile(newCert)
	if err := os.WriteFile(cert, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := manager.Status(cfg).Certificates[0].DaysLeft; got != 45 {
		t.Fatalf("served certificate changed without apply: %d days", got)
	}
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer occupied.Close()
	failed := cfg
	failed.HTTPSPort = occupied.Addr().(*net.TCPAddr).Port
	if err := os.WriteFile(cert, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := manager.Apply(context.Background(), failed, backend); err == nil || !strings.Contains(err.Error(), "HTTPS listener") {
		t.Fatalf("apply error = %v", err)
	}
	if !manager.Status(cfg).Running || get("/sub/token") != "ok" {
		t.Fatal("failed apply did not restore the working gateway")
	}
}

func TestAutomaticCertificateLifetimeWhileStopped(t *testing.T) {
	cert, _, _ := testCertificate(t, "auto.example.com", 12)
	data, err := os.ReadFile(cert)
	if err != nil {
		t.Fatal(err)
	}
	cache := t.TempDir()
	storage := &certmagic.FileStorage{Path: cache}
	issuer := &certmagic.ACMEIssuer{CA: certmagic.LetsEncryptProductionCA}
	if err := storage.Store(context.Background(), certmagic.StorageKeys.SiteCert(issuer.IssuerKey(), "auto.example.com"), data); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Domains = []DomainFrontDomain{{Name: "auto.example.com", Mode: "direct", Enabled: true, TLS: "auto"}}
	status := NewManager(cache).Status(cfg)
	if status.Running || status.Certificates[0].DaysLeft != 12 || status.Certificates[0].AutoRenew || status.Certificates[0].State != "renewal_due" {
		t.Fatalf("status = %#v", status)
	}
}

func TestGRPCGatewayForwardsHTTP2CleartextAndTrailers(t *testing.T) {
	origin := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor != 2 {
			http.Error(w, "HTTP/2 required", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/grpc")
		w.Header().Set("Trailer", "Grpc-Status")
		_, _ = w.Write([]byte{0, 0, 0, 0, 0})
		w.Header().Set("Grpc-Status", "0")
	}))
	origin.Config.Protocols = new(http.Protocols)
	origin.Config.Protocols.SetUnencryptedHTTP2(true)
	origin.Start()
	defer origin.Close()
	r := &frontRuntime{cfg: DomainFrontConfig{Domains: []DomainFrontDomain{{Name: "grpc.example.com", Enabled: true, Mode: "direct"}}}, backend: Backend{SubscriptionTarget: origin.URL, Routes: map[string][]Route{"grpc.example.com": {{Path: "/service", Target: origin.URL, H2C: true}}}}}
	handler, err := r.handler()
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(handler)
	defer front.Close()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, front.URL+"/service/Method", strings.NewReader("frame"))
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "grpc.example.com"
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("TE", "trailers")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK || len(body) != 5 || resp.Trailer.Get("Grpc-Status") != "0" {
		t.Fatalf("gRPC response = %d %v %#v headers=%#v", resp.StatusCode, body, resp.Trailer, resp.Header)
	}
}

func TestWebSocketGatewayPreservesUpgrade(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = buffer.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		_ = buffer.Flush()
		line, err := buffer.ReadString('\n')
		if err == nil {
			_, _ = buffer.WriteString(line)
			_ = buffer.Flush()
		}
	}))
	defer origin.Close()
	r := &frontRuntime{cfg: DomainFrontConfig{Domains: []DomainFrontDomain{{Name: "ws.example.com", Enabled: true, Mode: "direct"}}}, backend: Backend{SubscriptionTarget: origin.URL, Routes: map[string][]Route{"ws.example.com": {{Path: "/ws", Target: origin.URL}}}}}
	handler, err := r.handler()
	if err != nil {
		t.Fatal(err)
	}
	front := httptest.NewServer(handler)
	defer front.Close()
	conn, err := net.DialTimeout("tcp", front.Listener.Addr().String(), 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(conn, "GET /ws HTTP/1.1\r\nHost: ws.example.com\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade status = %d", response.StatusCode)
	}
	_, _ = io.WriteString(conn, "ping\n")
	line, err := reader.ReadString('\n')
	if err != nil || line != "ping\n" {
		t.Fatalf("upgrade echo = %q %v", line, err)
	}
}

func TestReconfigureThroughGatewayKeepsApplyResponseAlive(t *testing.T) {
	cert, key, roots := testCertificate(t, "panel.example.com", 45)
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Listen = "127.0.0.1"
	cfg.HTTPPort = unusedPort(t)
	cfg.HTTPSPort = unusedPort(t)
	cfg.PublicPort = cfg.HTTPSPort
	cfg.Domains = []DomainFrontDomain{{Name: "panel.example.com", Mode: "direct", Enabled: true, Panel: true, TLS: "manual", CertFile: cert, KeyFile: key}}
	manager := NewManager(t.TempDir())
	t.Cleanup(manager.Stop)
	var backend Backend
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := manager.Apply(r.Context(), cfg, backend); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = io.WriteString(w, "applied")
	}))
	defer origin.Close()
	backend = Backend{PanelTarget: origin.URL, PanelPath: "/panel/", SubscriptionTarget: origin.URL, SubscriptionPaths: []string{"/sub/"}}
	if err := manager.Apply(context.Background(), cfg, backend); err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "panel.example.com", MinVersion: tls.VersionTLS12}, ForceAttemptHTTP2: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, fmt.Sprintf("https://127.0.0.1:%d/panel/apply", cfg.HTTPSPort), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "panel.example.com"
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("apply response was interrupted: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK || string(body) != "applied" {
		t.Fatalf("apply response = %d %q %v", resp.StatusCode, body, err)
	}
}

func TestCachedAutomaticCertificateStartsWithoutReissuing(t *testing.T) {
	for _, mode := range []string{"auto", "certmagic"} {
		t.Run(mode, func(t *testing.T) { testCachedAutomaticCertificate(t, mode) })
	}
}

func testCachedAutomaticCertificate(t *testing.T, mode string) {
	t.Helper()
	certFile, keyFile, roots := testCertificate(t, "auto.example.com", 90)
	certPEM, err := os.ReadFile(certFile)
	if err != nil {
		t.Fatal(err)
	}
	keyPEM, err := os.ReadFile(keyFile)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	storage := &certmagic.FileStorage{Path: dir}
	issuer := &certmagic.ACMEIssuer{CA: certmagic.LetsEncryptProductionCA}
	for key, value := range map[string][]byte{
		certmagic.StorageKeys.SiteCert(issuer.IssuerKey(), "auto.example.com"):       certPEM,
		certmagic.StorageKeys.SitePrivateKey(issuer.IssuerKey(), "auto.example.com"): keyPEM,
		certmagic.StorageKeys.SiteMeta(issuer.IssuerKey(), "auto.example.com"):       []byte(`{"sans":["auto.example.com"]}`),
	} {
		if err := storage.Store(context.Background(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "cached") }))
	defer origin.Close()
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.AcceptTerms = true
	cfg.Listen = "127.0.0.1"
	cfg.HTTPPort = unusedPort(t)
	cfg.HTTPSPort = unusedPort(t)
	cfg.PublicPort = cfg.HTTPSPort
	cfg.Domains = []DomainFrontDomain{{Name: "auto.example.com", Mode: "sub_link_only", Enabled: true, TLS: mode}}
	manager := NewManager(dir)
	defer manager.Stop()
	reuseCert, reuseKey, _ := testCertificate(t, "reuse.example.com", 45)
	cfg.Domains = append(cfg.Domains, DomainFrontDomain{Name: "reuse.example.com", Mode: "sub_link_only", Enabled: true, TLS: "existing"})
	files := map[string]CertificateFiles{"reuse.example.com": {CertFile: reuseCert, KeyFile: reuseKey, Source: "panel", Renewal: "acme.sh", AutoRenew: true}}
	if mode == "certmagic" {
		files["auto.example.com"] = CertificateFiles{CertFile: "must-not-use", KeyFile: "must-not-use", Source: "panel"}
	}
	if err := manager.Apply(context.Background(), cfg, Backend{SubscriptionTarget: origin.URL, SubscriptionPaths: []string{"/sub/"}, Certificates: files}); err != nil {
		t.Fatal(err)
	}
	if got := len(manager.runtime.cache.AllMatchingCertificates("auto.example.com")); got == 0 {
		t.Fatal("saved certificate must be ready before Apply returns")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "auto.example.com", MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d/sub/id", cfg.HTTPSPort), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "auto.example.com"
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != http.StatusOK || string(body) != "cached" {
		t.Fatalf("cached TLS response = %d %q", response.StatusCode, body)
	}
	status := manager.Status(cfg)
	if manager.runtime.external["auto.example.com"] != nil || status.Certificates[0].Source != "gateway" || status.Certificates[0].Renewal != "certmagic" {
		t.Fatalf("new domain is not managed by CertMagic: %#v", status.Certificates[0])
	}
	if status.Certificates[1].Source != "panel" || status.Certificates[1].DaysLeft != 45 || !status.Certificates[1].AutoRenew {
		t.Fatalf("mixed external renewal lost: %#v", status.Certificates[1])
	}
	if !status.Certificates[0].AutoRenew || status.Certificates[0].DaysLeft != 90 || status.Certificates[0].State != "valid" {
		t.Fatalf("cached auto TLS = %#v", status)
	}
}

func TestInvalidCachedCertificateReleasesGatewayPorts(t *testing.T) {
	dir := t.TempDir()
	storage := &certmagic.FileStorage{Path: dir}
	issuer := &certmagic.ACMEIssuer{CA: certmagic.LetsEncryptProductionCA}
	name := "broken.example.com"
	for key, value := range map[string][]byte{
		certmagic.StorageKeys.SiteCert(issuer.IssuerKey(), name):       []byte("invalid certificate"),
		certmagic.StorageKeys.SitePrivateKey(issuer.IssuerKey(), name): []byte("invalid private key"),
		certmagic.StorageKeys.SiteMeta(issuer.IssuerKey(), name):       []byte(`{"sans":["broken.example.com"]}`),
	} {
		if err := storage.Store(context.Background(), key, value); err != nil {
			t.Fatal(err)
		}
	}
	cfg := DefaultConfig()
	cfg.Enabled, cfg.AcceptTerms = true, true
	cfg.Listen = "127.0.0.1"
	cfg.HTTPPort, cfg.HTTPSPort = unusedPort(t), unusedPort(t)
	cfg.PublicPort = cfg.HTTPSPort
	cfg.Domains = []DomainFrontDomain{{Name: name, Mode: "sub_link_only", Enabled: true, TLS: "certmagic"}}
	manager := NewManager(dir)
	defer manager.Stop()
	if err := manager.Apply(context.Background(), cfg, Backend{}); err == nil || !strings.Contains(err.Error(), "cached certificate") {
		t.Fatalf("invalid cached certificate error = %v", err)
	}
	for _, port := range []int{cfg.HTTPPort, cfg.HTTPSPort} {
		listener, err := net.Listen("tcp", net.JoinHostPort(cfg.Listen, strconv.Itoa(port)))
		if err != nil {
			t.Fatalf("failed startup leaked port %d: %v", port, err)
		}
		_ = listener.Close()
	}
}
