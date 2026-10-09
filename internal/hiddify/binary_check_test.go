package hiddify

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPinnedBinaryChecksGeneratedConfigurations(t *testing.T) {
	binary := os.Getenv("HIDDIFY_CONFIG_CHECK_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_CONFIG_CHECK_BINARY to the panel build; no network listeners are started")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) ([]byte, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir = filepath.Dir(binary)
		return cmd.CombinedOutput()
	}
	version, err := run("version")
	if err != nil || !strings.Contains(string(version), "hiddify-core version "+TargetVersion) || !strings.Contains(string(version), "with_v2ray_api") {
		t.Fatalf("not the pinned panel build: %v: %s", err, version)
	}
	pairOutput, err := run("vlessenc", "--json")
	if err != nil {
		t.Fatalf("generate real encryption keys: %v: %s", err, pairOutput)
	}
	var pair struct {
		Encryption string `json:"encryption"`
		Decryption string `json:"decryption"`
	}
	if err := json.Unmarshal(pairOutput, &pair); err != nil || pair.Decryption == "" || pair.Encryption == "" {
		t.Fatalf("invalid encryption pair: %v: %s", err, pairOutput)
	}
	dir := t.TempDir()
	cert, key := testCertificate(t, dir)
	checked := func(t *testing.T, cfg map[string]any) {
		t.Helper()
		data, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, "check.json")
		if err := os.WriteFile(file, data, 0o600); err != nil {
			t.Fatal(err)
		}
		out, err := run("check", "-c", file)
		if err != nil {
			t.Fatalf("native schema/constructor rejected generated config: %v: %s", err, out)
		}
	}
	t.Run("encrypted XHTTP inbound and user stats", func(t *testing.T) {
		raw := vlessInbound()
		object(raw, "settings")["decryption"] = pair.Decryption
		inbound, err := TranslateInbound(raw)
		if err != nil {
			t.Fatal(err)
		}
		checked(t, map[string]any{"inbounds": []any{inbound}, "experimental": map[string]any{
			"v2ray_api": map[string]any{"listen": "127.0.0.1:0", "stats": map[string]any{"enabled": true, "users": []string{"alice"}}},
		}})
	})
	t.Run("Snell6 users", func(t *testing.T) {
		inbound, err := TranslateInbound(map[string]any{
			"protocol": "snell", "tag": "snell", "listen": "127.0.0.1", "port": 443,
			"settings": map[string]any{"version": 6, "psk": "test-psk-12345678", "mode": "unshaped", "clients": []any{
				map[string]any{"email": "alice", "password": strings.Repeat("11", 32), "enable": true},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		checked(t, map[string]any{"inbounds": []any{inbound}})
	})
	t.Run("encrypted outbound with FinalMask", func(t *testing.T) {
		outbound, err := TranslateOutbound(map[string]any{
			"protocol": "vless", "tag": "out", "settings": map[string]any{
				"address": "example.com", "port": 443, "id": "11111111-1111-4111-8111-111111111111", "encryption": pair.Encryption,
			}, "streamSettings": map[string]any{"network": "tcp", "finalmask": map[string]any{
				"tcp": []any{map[string]any{"type": "fragment", "settings": map[string]any{"packets": "tlshello", "length": "10-20", "delay": "0"}}},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		checked(t, map[string]any{"outbounds": []any{outbound}})
	})
	t.Run("MASQUE server TLS", func(t *testing.T) {
		endpoint, err := TranslateMASQUEEndpoint(map[string]any{
			"protocol": "masque", "tag": "masque", "port": 443, "listen": "127.0.0.1",
			"settings": map[string]any{"version": []int{3}, "address": []string{"10.123.0.1/24"}, "clients": []any{
				map[string]any{"email": "alice", "password": "test-password", "enable": true},
			}, "tls": map[string]any{"certificatePath": cert, "keyPath": key}},
		})
		if err != nil {
			t.Fatal(err)
		}
		checked(t, map[string]any{"endpoints": []any{endpoint}})
	})
	t.Run("XHTTP separate download TLS", func(t *testing.T) {
		outbound, err := TranslateOutbound(map[string]any{
			"protocol": "vless", "tag": "out", "settings": map[string]any{
				"address": "up.example.com", "port": 443, "id": "11111111-1111-4111-8111-111111111111",
			}, "streamSettings": map[string]any{"network": "xhttp", "xhttpSettings": map[string]any{
				"mode": "stream-up", "path": "/up", "downloadSettings": map[string]any{
					"address": "down.example.com", "port": 443, "network": "xhttp", "security": "tls",
					"tlsSettings":   map[string]any{"serverName": "down.example.com"},
					"xhttpSettings": map[string]any{"path": "/down", "mode": "auto", "xPaddingBytes": "100-200"},
				},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		checked(t, map[string]any{"outbounds": []any{outbound}})
	})
	t.Run("Hysteria2 gecko TLS", func(t *testing.T) {
		inbound, err := TranslateInbound(map[string]any{
			"protocol": "hysteria", "tag": "hy2", "port": 443, "listen": "127.0.0.1",
			"settings": map[string]any{"clients": []any{map[string]any{"email": "alice", "password": "test-password"}}},
			"streamSettings": map[string]any{
				"network": "hysteria", "security": "tls", "hysteriaSettings": map[string]any{"version": 2},
				"tlsSettings": map[string]any{"certificates": []any{map[string]any{"certificateFile": cert, "keyFile": key}}},
				"finalmask":   map[string]any{"udp": []any{map[string]any{"type": "salamander", "settings": map[string]any{"password": "test-obfs", "packetSize": "100-200"}}}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		checked(t, map[string]any{"inbounds": []any{inbound}})
	})
	t.Run("explicit config required", func(t *testing.T) {
		out, err := run("check")
		if err == nil || !strings.Contains(string(out), "check requires --config or --config-directory") {
			t.Fatalf("check without candidate succeeded: %v: %s", err, out)
		}
	})
}

func testCertificate(t *testing.T, dir string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"},
		NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	private, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	for path, block := range map[string]*pem.Block{certPath: {Type: "CERTIFICATE", Bytes: der}, keyPath: {Type: "EC PRIVATE KEY", Bytes: private}} {
		if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return certPath, keyPath
}
