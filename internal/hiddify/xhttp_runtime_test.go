package hiddify

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"log"
	"maps"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

// Exercise translated panel configs through a real core, including multiple
// HTTP requests and a body larger than one packet-up chunk in both directions.
func TestXHTTPRuntimeTransfer(t *testing.T) {
	binary := os.Getenv("HIDDIFY_RUNTIME_BINARY")
	xrayBinary := os.Getenv("XRAY_RUNTIME_BINARY")
	xray := xrayBinary != ""
	if xray {
		binary = xrayBinary
	}
	if binary == "" {
		t.Skip("set HIDDIFY_RUNTIME_BINARY or XRAY_RUNTIME_BINARY to test XHTTP loopback transfers")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	for _, security := range []string{"none", "tls", "reality"} {
		if security == "reality" && !xray {
			continue
		}
		for _, mode := range []string{"auto", "packet-up", "stream-up", "stream-one"} {
			if security == "none" && mode != "auto" && mode != "packet-up" {
				continue
			}
			for _, split := range []bool{false, true} {
				if split && mode == "stream-one" {
					continue
				}
				name := security + "/" + mode
				if split {
					name += "/split-download"
				}
				t.Run(name, func(t *testing.T) {
					dir := t.TempDir()
					reserve := func() (string, int) {
						l, err := net.Listen("tcp", "127.0.0.1:0")
						if err != nil {
							t.Fatal(err)
						}
						addr, port := l.Addr().String(), l.Addr().(*net.TCPAddr).Port
						_ = l.Close()
						return addr, port
					}
					_, port := reserve()
					socksAddr, socksPort := reserve()
					stream := map[string]any{"network": "xhttp", "security": security, "xhttpSettings": map[string]any{"path": "/audit", "mode": mode, "scMaxEachPostBytes": "32768"}}
					serverStream := map[string]any{"network": "xhttp", "security": security, "xhttpSettings": map[string]any{"path": "/audit", "mode": "auto", "scMaxEachPostBytes": "32768"}}
					if security == "tls" {
						cert, key := testCertificate(t, dir)
						serverStream["tlsSettings"] = map[string]any{"certificates": []any{map[string]any{"certificateFile": cert, "keyFile": key}}}
						if xray {
							// Xray accepts full-certificate SHA-256 pinning.
							data, err := os.ReadFile(cert)
							if err != nil {
								t.Fatal(err)
							}
							block, _ := pem.Decode(data)
							hash := sha256.Sum256(block.Bytes)
							stream["tlsSettings"] = map[string]any{"serverName": "localhost", "pinnedPeerCertSha256": hex.EncodeToString(hash[:]), "alpn": []any{"h2"}}
						} else {
							// Pinned sing-box does not implement Xray's full-certificate
							// pin. Trust the fixture CA explicitly without disabling
							// peer verification.
							stream["tlsSettings"] = map[string]any{
								"serverName": "localhost", "fingerprint": "unsafe", "alpn": []any{"h2"},
								"certificates": []any{map[string]any{"usage": "verify", "certificateFile": cert}},
							}
						}
					}
					if security == "reality" {
						target := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
						target.EnableHTTP2 = true
						target.Config.ErrorLog = log.New(io.Discard, "", 0)
						target.StartTLS()
						defer target.Close()
						private, err := ecdh.X25519().GenerateKey(rand.Reader)
						if err != nil {
							t.Fatal(err)
						}
						serverStream["realitySettings"] = map[string]any{"target": target.Listener.Addr().String(), "serverNames": []any{"localhost"}, "privateKey": base64.RawURLEncoding.EncodeToString(private.Bytes()), "shortIds": []any{"abcd"}}
						stream["realitySettings"] = map[string]any{"serverName": "localhost", "password": base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()), "shortId": "abcd", "fingerprint": "chrome"}
					}
					if split {
						download := map[string]any{"address": "127.0.0.1", "port": port, "network": "xhttp", "security": security, "xhttpSettings": map[string]any{"path": "/audit", "mode": "auto"}}
						if value := stream["tlsSettings"]; value != nil {
							download["tlsSettings"] = value
						}
						if value := stream["realitySettings"]; value != nil {
							download["realitySettings"] = value
						}
						settings := maps.Clone(stream["xhttpSettings"].(map[string]any))
						settings["downloadSettings"] = download
						stream["xhttpSettings"] = settings
					}
					const uuid = "11111111-1111-4111-8111-111111111111"
					inbound, err := TranslateInbound(map[string]any{"protocol": "vless", "tag": "xhttp", "listen": "127.0.0.1", "port": port, "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": uuid, "email": "audit", "enable": true}}}, "streamSettings": serverStream})
					if err != nil {
						t.Fatal(err)
					}
					outbound, err := TranslateOutbound(map[string]any{"protocol": "vless", "tag": "xhttp", "settings": map[string]any{"address": "127.0.0.1", "port": port, "id": uuid}, "streamSettings": stream})
					if err != nil && !xray {
						t.Fatal(err)
					}
					start := func(name string, cfg map[string]any, ready string) {
						if xray {
							cfg["log"] = map[string]any{"loglevel": "info"}
						}
						data, err := json.Marshal(cfg)
						if err != nil {
							t.Fatal(err)
						}
						file := filepath.Join(dir, name+".json")
						if err := os.WriteFile(file, data, 0o600); err != nil {
							t.Fatal(err)
						}
						log, err := os.Create(filepath.Join(dir, name+".log"))
						if err != nil {
							t.Fatal(err)
						}
						command := "srun"
						if xray {
							command = "run"
						}
						cmd := exec.Command(binary, command, "-c", file)
						cmd.Stdout, cmd.Stderr = log, log
						if err := cmd.Start(); err != nil {
							t.Fatal(err)
						}
						t.Cleanup(func() {
							_ = cmd.Process.Kill()
							_ = cmd.Wait()
							_ = log.Close()
							if t.Failed() {
								data, _ := os.ReadFile(log.Name())
								t.Log(string(data))
							}
						})
						deadline := time.Now().Add(5 * time.Second)
						for time.Now().Before(deadline) {
							conn, err := net.DialTimeout("tcp", ready, 100*time.Millisecond)
							if err == nil {
								_ = conn.Close()
								return
							}
							time.Sleep(20 * time.Millisecond)
						}
						t.Fatal("core did not open listener")
					}
					serverConfig := map[string]any{"inbounds": []any{inbound}, "outbounds": []any{map[string]any{"type": "direct"}}}
					clientConfig := map[string]any{"inbounds": []any{map[string]any{"type": "socks", "listen": "127.0.0.1", "listen_port": socksPort}}, "outbounds": []any{outbound}}
					if xray {
						serverConfig = map[string]any{"inbounds": []any{map[string]any{"protocol": "vless", "listen": "127.0.0.1", "port": port, "settings": map[string]any{"decryption": "none", "clients": []any{map[string]any{"id": uuid}}}, "streamSettings": serverStream}}, "outbounds": []any{map[string]any{"protocol": "freedom", "settings": map[string]any{"finalRules": []any{map[string]any{"action": "allow", "ip": []any{"127.0.0.1/32"}}}}}}}
						clientConfig = map[string]any{"inbounds": []any{map[string]any{"protocol": "socks", "listen": "127.0.0.1", "port": socksPort, "settings": map[string]any{"auth": "noauth"}}}, "outbounds": []any{map[string]any{"protocol": "vless", "settings": map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": port, "users": []any{map[string]any{"id": uuid, "encryption": "none"}}}}}, "streamSettings": stream}}}
					}
					if !xray {
						checkProtectionConfig(t, "inbounds", inbound)
						checkProtectionConfig(t, "outbounds", outbound)
					}
					start("server", serverConfig, net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
					start("client", clientConfig, socksAddr)
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(w, r.Body) }))
					defer upstream.Close()
					dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
					if err != nil {
						t.Fatal(err)
					}
					transport := &http.Transport{DialContext: dialer.(proxy.ContextDialer).DialContext}
					defer transport.CloseIdleConnections()
					client := &http.Client{Transport: transport, Timeout: 15 * time.Second}
					payload := bytes.Repeat([]byte("xhttp-transfer-"), 32768)
					for i := 0; i < 3; i++ {
						ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
						req, _ := http.NewRequestWithContext(ctx, http.MethodPost, upstream.URL, bytes.NewReader(payload))
						res, err := client.Do(req)
						if err != nil {
							cancel()
							t.Fatal(err)
						}
						body, err := io.ReadAll(res.Body)
						_ = res.Body.Close()
						cancel()
						if err != nil || !bytes.Equal(body, payload) {
							t.Fatalf("transfer %d: received %d of %d bytes: %v", i, len(body), len(payload), err)
						}
					}
				})
			}
		}
	}
}
