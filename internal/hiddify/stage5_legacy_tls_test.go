package hiddify

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/net/proxy"
)

func TestStage5ExplicitLegacyCipherTraffic(t *testing.T) {
	binary := os.Getenv("HIDDIFY_RUNTIME_BINARY")
	if binary == "" {
		t.Skip("set HIDDIFY_RUNTIME_BINARY for explicit legacy cipher handshake")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{SerialNumber: big.NewInt(5), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPath, keyPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatal(err)
	}
	const suite = "TLS_RSA_WITH_AES_128_GCM_SHA256"
	const uuid = "11111111-1111-4111-8111-111111111111"
	port, socksPort := stage5Port(t), stage5Port(t)
	server, err := TranslateInbound(map[string]any{"protocol": "vless", "tag": "server", "listen": "127.0.0.1", "port": port, "settings": map[string]any{"clients": []any{map[string]any{"id": uuid, "email": "legacy", "enable": true}}, "decryption": "none"}, "streamSettings": map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{"minVersion": "1.2", "maxVersion": "1.2", "cipherSuites": suite, "certificates": []any{map[string]any{"certificateFile": certPath, "keyFile": keyPath}}}}})
	if err != nil {
		t.Fatal(err)
	}
	client, err := TranslateOutbound(map[string]any{"protocol": "vless", "tag": "proxy", "settings": map[string]any{"address": "127.0.0.1", "port": port, "id": uuid}, "streamSettings": map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{"serverName": "localhost", "fingerprint": "unsafe", "minVersion": "1.2", "maxVersion": "1.2", "cipherSuites": suite, "certificates": []any{map[string]any{"usage": "verify", "certificateFile": certPath}}}}})
	if err != nil {
		t.Fatal(err)
	}
	stage5Start(t, binary, dir, "server", map[string]any{"inbounds": []any{server}, "outbounds": []any{map[string]any{"type": "direct"}}}, port, true)
	stage5Start(t, binary, dir, "client", map[string]any{"inbounds": []any{map[string]any{"type": "socks", "listen": "127.0.0.1", "listen_port": socksPort}}, "outbounds": []any{client}}, socksPort, true)
	dialer, err := proxy.SOCKS5("tcp", fmt.Sprintf("127.0.0.1:%d", socksPort), nil, proxy.Direct)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := dialer.(proxy.ContextDialer).DialContext(ctx, "tcp", stage5Echo(t, "127.0.0.1:0"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
	const payload = "explicit legacy TLS handshake"
	if _, err := io.WriteString(conn, payload); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(payload))
	if _, err := io.ReadFull(conn, got); err != nil {
		t.Fatal(err)
	}
	if string(got) != payload {
		t.Fatal("legacy TLS transfer corrupted")
	}
}
