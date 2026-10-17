package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTelemtWebCarrierSelection(t *testing.T) {
	base := "[access.users]\nxui = \"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\"\n"
	state := TelemtWebProxyState{Enabled: true, Domain: "example.com", Secret: "0123456789abcdef0123456789abcdef", PublicAddr: "203.0.113.1:443", Carrier: "https", Carriers: []string{"websocket-lanes", "websocket", "https"}}
	got, err := appendTelemtWebProxyConfig(base, state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `carriers = ["websocket-lanes", "websocket", "https"]`) || !strings.Contains(got, `carrier = "https"`) {
		t.Fatalf("carrier order/fallback lost: %s", got)
	}
	for _, bad := range [][]string{{"websocket", "websocket"}, {"invalid"}} {
		state.Carriers = bad
		if _, err := appendTelemtWebProxyConfig(base, state); err == nil {
			t.Fatalf("accepted invalid carriers %v", bad)
		}
	}
	state.Carriers = nil
	state.Carrier = "\"\ninjected=true"
	if _, err := appendTelemtWebProxyConfig(base, state); err == nil {
		t.Fatal("accepted TOML injection")
	}
}

func TestTelemtWebAlternativeFrontends(t *testing.T) {
	for _, front := range []string{"caddy", "haproxy"} {
		t.Run(front, func(t *testing.T) {
			binary := os.Getenv(strings.ToUpper(front) + "_TEST_BINARY")
			if binary == "" {
				t.Skip("set " + strings.ToUpper(front) + "_TEST_BINARY for native frontend integration")
			}
			dir := t.TempDir()
			sub := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Forwarded-For") != "127.0.0.1" || r.Header.Get("X-Forwarded-Proto") != "https" {
					http.Error(w, "forged forwarding headers", http.StatusBadRequest)
					return
				}
				fmt.Fprint(w, r.Host+" "+r.URL.RequestURI())
			}))
			defer sub.Close()
			certificate := sub.TLS.Certificates[0]
			leaf, err := x509.ParseCertificate(certificate.Certificate[0])
			if err != nil {
				t.Fatal(err)
			}
			key, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
			if err != nil {
				t.Fatal(err)
			}
			certData := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]})
			keyData := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key})
			certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
			if err := os.WriteFile(certPath, certData, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(keyPath, keyData, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "tls.pem"), append(certData, keyData...), 0o600); err != nil {
				t.Fatal(err)
			}
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Upgrade") != "websocket" {
					fmt.Fprint(w, "web-decoy")
					return
				}
				conn, rw, err := w.(http.Hijacker).Hijack()
				if err != nil {
					t.Error(err)
					return
				}
				defer conn.Close()
				fmt.Fprint(rw, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: websocket\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n")
				if err := rw.Flush(); err != nil {
					return
				}
				input := make([]byte, 4)
				if _, err := io.ReadFull(rw, input); err != nil {
					return
				}
				if string(input) == "ping" {
					fmt.Fprint(conn, "pong")
				}
			}))
			defer backend.Close()
			_, backendPort, _ := net.SplitHostPort(strings.TrimPrefix(backend.URL, "http://"))
			port, _ := strconv.Atoi(backendPort)
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			publicAddr := listener.Addr().String()
			_ = listener.Close()
			state := TelemtWebProxyState{Frontend: front, Domain: "example.com", ListenPort: port, CertFile: certPath, KeyFile: keyPath}
			data, err := telemtWebFrontendConfig(state, telemtWebSubscriptionTarget{Paths: []string{"/ImportedRoute/*"}, Upstream: sub.URL, TLSCAFile: certPath, TLSServerName: "example.com"})
			if err != nil {
				t.Fatal(err)
			}
			config := strings.ReplaceAll(string(data), telemtWebFrontDir, dir)
			config = strings.ReplaceAll(config, ":443", publicAddr)
			config = strings.ReplaceAll(config, "unix//run/3x-ui-telemt-front/caddy.sock", "127.0.0.1:0")
			path := filepath.Join(dir, "config")
			if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
				t.Fatal(err)
			}
			args := []string{"run", "--config", path}
			if front == "haproxy" {
				args = []string{"-db", "-f", path}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, args...)
			logFile, err := os.Create(filepath.Join(dir, "log"))
			if err != nil {
				t.Fatal(err)
			}
			defer logFile.Close()
			cmd.Stdout = logFile
			cmd.Stderr = logFile
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
			roots := x509.NewCertPool()
			roots.AddCert(leaf)
			client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, ServerName: "example.com", MinVersion: tls.VersionTLS12}}}
			defer client.CloseIdleConnections()
			request := func(path, host string, upgrade bool) (*http.Response, error) {
				req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+publicAddr+path, nil)
				if err != nil {
					return nil, err
				}
				req.Host = host
				req.Header.Set("X-Forwarded-For", "198.51.100.1")
				if upgrade {
					req.Header.Set("Connection", "Upgrade")
					req.Header.Set("Upgrade", "websocket")
					req.Header.Set("Sec-WebSocket-Version", "13")
					req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
				}
				return client.Do(req)
			}
			deadline := time.Now().Add(5 * time.Second)
			for {
				resp, err := request("/", "example.com", false)
				if err == nil {
					_ = resp.Body.Close()
					break
				}
				if time.Now().After(deadline) {
					logs, _ := os.ReadFile(logFile.Name())
					t.Fatalf("frontend not ready: %v\n%s\n%s", err, logs, config)
				}
				time.Sleep(25 * time.Millisecond)
			}
			for _, tc := range []struct {
				path, host, body string
				status           int
			}{{"/ImportedRoute/client?format=json", "example.com", "example.com /ImportedRoute/client?format=json", 200}, {"/", "example.com", "web-decoy", 200}, {"/ImportedRoute/client", "other.example.com", "", 404}} {
				resp, err := request(tc.path, tc.host, false)
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode != tc.status || (tc.body != "" && string(body) != tc.body) {
					t.Fatalf("%s %s: got %d %q", tc.host, tc.path, resp.StatusCode, body)
				}
			}
			client.Timeout = 0
			resp, err := request("/ws", "example.com", true)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusSwitchingProtocols {
				t.Fatalf("WebSocket status %d", resp.StatusCode)
			}
			rw, ok := resp.Body.(io.ReadWriteCloser)
			if !ok {
				t.Fatal("upgrade is not bidirectional")
			}
			if _, err := rw.Write([]byte("ping")); err != nil {
				t.Fatal(err)
			}
			reply := make([]byte, 4)
			if _, err := io.ReadFull(rw, reply); err != nil {
				t.Fatal(err)
			}
			if string(reply) != "pong" {
				t.Fatalf("WebSocket reply: %q", reply)
			}
		})
	}
}

func TestTelemtWebRenewalRebuildsHAProxyCertificateSafely(t *testing.T) {
	dir := t.TempDir()
	cert := filepath.Join(dir, "cert ' $(touch injected).pem")
	key := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(cert, []byte("certificate\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(key, []byte("private-key\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" > \"$TEST_RELOAD_LOG\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	script := renderTelemtWebRenewalHook(TelemtWebProxyState{Frontend: "haproxy", CertFile: cert, KeyFile: key})
	script = strings.ReplaceAll(script, telemtWebFrontDir, dir)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "TEST_RELOAD_LOG="+filepath.Join(dir, "reload"))
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("renewal hook: %v %s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(dir, "tls.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "certificate\nprivate-key\n" {
		t.Fatalf("renewed PEM = %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "injected")); !os.IsNotExist(err) {
		t.Fatal("certificate path was evaluated by the shell")
	}
	reload, err := os.ReadFile(filepath.Join(dir, "reload"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(reload)) != "try-reload-or-restart "+telemtWebFrontUnit {
		t.Fatalf("wrong service reload: %s", reload)
	}
}

func TestTelemtWebFrontendStopsOnlyOwnedService(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte("#!/bin/sh\nprintf '%s\\n' \"$*\" >> \"$TEST_SERVICE_LOG\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
	t.Setenv("TEST_SERVICE_LOG", filepath.Join(dir, "calls"))
	for _, frontend := range []string{"caddy", "haproxy"} {
		if err := telemtWebStopFrontend(TelemtWebProxyState{Frontend: frontend}, true); err != nil {
			t.Fatal(err)
		}
	}
	calls, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	want := "stop " + telemtWebFrontUnit + "\ndisable " + telemtWebFrontUnit + "\n"
	if string(calls) != want+want {
		t.Fatalf("unrelated service affected: %s", calls)
	}
}

func TestTelemtWebCertificateRenewalSwitching(t *testing.T) {
	for _, tc := range []struct{ name, data, frontend, want string }{
		{"managed nginx to caddy", "[renewalparams]\nauthenticator = webroot\nwebroot_path = /var/lib/x-ui/telemt-web,\n", "caddy", "reconfigure --cert-name web.example.com --non-interactive --standalone"},
		{"managed map to haproxy", "[renewalparams]\nauthenticator = webroot\n[[webroot_map]]\nweb.example.com = /var/lib/x-ui/telemt-web\n", "haproxy", "reconfigure --cert-name web.example.com --non-interactive --standalone"},
		{"standalone to nginx", "[renewalparams]\nauthenticator = standalone\n", "nginx", "reconfigure --cert-name web.example.com --non-interactive --webroot --webroot-path /var/lib/x-ui/telemt-web"},
		{"foreign webroot", "authenticator = webroot\nwebroot_path = /var/www/other,\n", "caddy", ""},
		{"similar path is foreign", "authenticator = webroot\nwebroot_path = /var/lib/x-ui/telemt-web-other,\n", "caddy", ""},
		{"comment is not ownership", "authenticator = webroot\n# webroot_path = /var/lib/x-ui/telemt-web\nwebroot_path = /var/www/other\n", "caddy", ""},
		{"DNS managed externally", "authenticator = dns-cloudflare\nwebroot_path = /var/lib/x-ui/telemt-web\n", "nginx", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.Join(telemtWebRenewalArguments([]byte(tc.data), tc.frontend, "web.example.com"), " ")
			if got != tc.want {
				t.Fatalf("renewal command %q, want %q", got, tc.want)
			}
		})
	}
	if got := normalizeTelemtWebDomain(" HTTPS://Web.Example.COM/ "); got != "web.example.com" {
		t.Fatalf("domain normalization: %q", got)
	}
}
