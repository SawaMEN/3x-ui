package domainfront

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuditRouteRefreshPreservesCertificateLifecycle(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.Domains = []DomainFrontDomain{{Name: "lifecycle.example.com", Enabled: true, Mode: "sub_link_only"}}
	events := 0
	r := &frontRuntime{cfg: cfg, backend: Backend{
		DNS01Token:       "test-dns-credential",
		CertificateEvent: func(context.Context, string, map[string]any) error { events++; return nil },
	}, errors: map[string]string{}}
	manager := NewManager(t.TempDir())
	manager.runtime = r
	for i := 0; i < 3; i++ {
		if err := manager.RefreshRoutes(Backend{SubscriptionTarget: "http://127.0.0.1:1"}); err != nil {
			t.Fatal(err)
		}
		if err := r.event(context.Background(), "cert_renewed", map[string]any{"identifier": "lifecycle.example.com"}); err != nil {
			t.Fatal(err)
		}
		if events != i+1 {
			t.Errorf("route refresh removed certificate event delivery: events=%d, want %d", events, i+1)
		}
		if r.backend.DNS01Token != "test-dns-credential" {
			t.Error("route refresh discarded DNS-01 credential needed for runtime restoration")
		}
	}
}

func TestAuditGatewayClosesReplacedAndStoppedProxyPools(t *testing.T) {
	closed := make(chan struct{}, 8)
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed <- struct{}{}
		}
	}
	upstream.Start()
	defer upstream.Close()
	cert, key, roots := testCertificate(t, "pool.example.com", 45)
	cfg := DefaultConfig()
	cfg.Enabled, cfg.Listen = true, "127.0.0.1"
	cfg.HTTPPort, cfg.HTTPSPort = unusedPort(t), unusedPort(t)
	cfg.Domains = []DomainFrontDomain{{Name: "pool.example.com", Enabled: true, Mode: "sub_link_only", TLS: "existing"}}
	backend := Backend{
		SubscriptionTarget: upstream.URL, SubscriptionPaths: []string{"/sub/"},
		Certificates: map[string]CertificateFiles{"pool.example.com": {CertFile: cert, KeyFile: key}},
	}
	manager := NewManager(t.TempDir())
	defer manager.Stop()
	if err := manager.Apply(context.Background(), cfg, backend); err != nil {
		t.Fatal(err)
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "pool.example.com"}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	request := func() {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("https://127.0.0.1:%d/sub/client", cfg.HTTPSPort), nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = "pool.example.com"
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK || string(body) != "ok" {
			t.Fatalf("proxy response: status=%d body=%q err=%v", response.StatusCode, body, err)
		}
	}
	expectClosed := func(action string) {
		t.Helper()
		select {
		case <-closed:
		case <-time.After(2 * time.Second):
			t.Errorf("%s retained its upstream idle connection", action)
		}
	}
	request()
	if err := manager.RefreshRoutes(backend); err != nil {
		t.Fatal(err)
	}
	expectClosed("route replacement")
	request()
	manager.Stop()
	expectClosed("gateway stop")
}

func TestAuditGatewayStopClosesUpgradedProxyConnection(t *testing.T) {
	done := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, buffer, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer close(done)
		defer conn.Close()
		_, _ = buffer.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
		_ = buffer.Flush()
		for {
			line, err := buffer.ReadString('\n')
			if err != nil {
				return
			}
			_, _ = buffer.WriteString(line)
			_ = buffer.Flush()
		}
	}))
	defer upstream.Close()
	cert, key, roots := testCertificate(t, "upgrade.example.com", 45)
	cfg := DefaultConfig()
	cfg.Enabled, cfg.Listen = true, "127.0.0.1"
	cfg.HTTPPort, cfg.HTTPSPort = unusedPort(t), unusedPort(t)
	cfg.Domains = []DomainFrontDomain{{Name: "upgrade.example.com", Enabled: true, Mode: "direct", TLS: "existing"}}
	backend := Backend{
		SubscriptionTarget: upstream.URL,
		Certificates:       map[string]CertificateFiles{"upgrade.example.com": {CertFile: cert, KeyFile: key}},
		Routes:             map[string][]Route{"upgrade.example.com": {{Path: "/ws", Target: upstream.URL}}},
	}
	manager := NewManager(t.TempDir())
	defer manager.Stop()
	if err := manager.Apply(context.Background(), cfg, backend); err != nil {
		t.Fatal(err)
	}
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 2 * time.Second}, "tcp", net.JoinHostPort("127.0.0.1", fmt.Sprint(cfg.HTTPSPort)), &tls.Config{RootCAs: roots, ServerName: "upgrade.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	_, _ = io.WriteString(conn, "GET /ws HTTP/1.1\r\nHost: upgrade.example.com\r\nConnection: Upgrade\r\nUpgrade: websocket\r\n\r\n")
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, nil)
	if err != nil || response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("upgrade response: %v, %v", response, err)
	}
	_, _ = io.WriteString(conn, "ping\n")
	if line, err := reader.ReadString('\n'); err != nil || line != "ping\n" {
		t.Fatalf("upgrade echo=%q err=%v", line, err)
	}
	if err := manager.RefreshRoutes(backend); err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(conn, "after refresh\n")
	if line, err := reader.ReadString('\n'); err != nil || line != "after refresh\n" {
		t.Fatalf("refresh interrupted active upgraded connection: %q, %v", line, err)
	}
	manager.Stop()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("gateway stop retained the upgraded upstream connection")
	}
	if _, err := reader.ReadByte(); err == nil {
		t.Fatal("gateway stop retained the upgraded client connection")
	} else {
		var timeout net.Error
		if errors.As(err, &timeout) && timeout.Timeout() {
			t.Fatal("upgraded client timed out instead of receiving connection closure")
		}
	}
}
