package oxidns

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestGenerateLocalAndDoH(t *testing.T) {
	cfg := Defaults()
	yaml, err := Generate(cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(yaml, "listen: \"127.0.0.1:15353\"") {
		t.Fatal("local DNS listener missing")
	}
	if !strings.Contains(yaml, "type: cache") || !strings.Contains(yaml, `      - exec: "$dns_cache"`) {
		t.Fatal("default DNS cache missing")
	}
	if strings.Contains(yaml, "http_server") {
		t.Fatal("DoH must stay off without credentials")
	}
	for _, v := range []string{"bootstrap: \"9.9.9.9:53\"", "type: udp_server", "type: tcp_server"} {
		if !strings.Contains(yaml, v) {
			t.Fatalf("missing %s", v)
		}
	}
}
func TestRejectUntrustedProvider(t *testing.T) {
	cfg := Defaults()
	cfg.Upstream = "foo\n    type: bad"
	if _, err := Generate(cfg, "", ""); err == nil {
		t.Fatal("unvalidated upstream accepted")
	}
}
func TestPreserveSingBoxDNSRules(t *testing.T) {
	original := map[string]any{"servers": []any{map[string]any{"tag": "google", "type": "https", "server": "dns.google"}}, "rules": []any{map[string]any{"domain_suffix": []string{"internal"}, "server": "google"}}, "final": "google", "strategy": "ipv4_only"}
	result, err := WithSingBoxDNS(original)
	if err != nil {
		t.Fatal(err)
	}
	if result["final"] != Tag {
		t.Fatal("wrong default")
	}
	if original["final"] != "google" {
		t.Fatal("modified input")
	}
	if !reflect.DeepEqual(result["rules"], original["rules"]) {
		t.Fatal("replaced routing rules")
	}
	if len(result["servers"].([]map[string]any)) != 2 {
		t.Fatal("user server lost")
	}
	if _, err := WithSingBoxDNS(result); err == nil {
		t.Fatal("must reject colliding managed tag")
	}
}

func TestDNSProviders(t *testing.T) {
	tests := []struct {
		name      string
		address   string
		bootstrap string
	}{
		{"google", "tls://dns.google:853", "8.8.8.8:53"},
		{"yandex-basic", "tls://common.dot.dns.yandex.net:853", "77.88.8.8:53"},
		{"yandex-safe", "tls://safe.dot.dns.yandex.net:853", "77.88.8.88:53"},
		{"yandex-family", "tls://family.dot.dns.yandex.net:853", "77.88.8.7:53"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Defaults()
			cfg.Upstream = tc.name
			address, bootstrap, valid := upstream(cfg)
			if !valid || address != tc.address || bootstrap != tc.bootstrap {
				t.Fatalf("unexpected upstream for %s: %q %q %v", tc.name, address, bootstrap, valid)
			}
			yaml, err := Generate(cfg, "", "")
			if err != nil {
				t.Fatal(err)
			}
			for _, item := range []string{tc.address, tc.bootstrap} {
				if !strings.Contains(yaml, item) {
					t.Fatalf("missing upstream value %s", item)
				}
			}
		})
	}
}

func TestGeneratedYAMLAgainstOfficialBinary(t *testing.T) {
	binary := os.Getenv("OXIDNS_CHECK_BINARY")
	if binary == "" {
		t.Skip("set OXIDNS_CHECK_BINARY to verify official OxiDNS parser")
	}
	for _, provider := range []string{"quad9", "cloudflare", "adguard", "google", "yandex-basic", "yandex-safe", "yandex-family"} {
		t.Run(provider, func(t *testing.T) {
			cfg := Defaults()
			cfg.Upstream = provider
			payload, err := Generate(cfg, "", "")
			if err != nil {
				t.Fatal(err)
			}
			workDir := t.TempDir()
			configFile := filepath.Join(workDir, "config.yaml")
			if err := os.WriteFile(configFile, []byte(payload), 0600); err != nil {
				t.Fatal(err)
			}
			command := exec.Command(binary, "check", "-c", configFile, "-d", workDir)
			if output, err := command.CombinedOutput(); err != nil {
				t.Fatalf("official OxiDNS rejected %s: %v\n%s\n%s", provider, err, output, payload)
			}
		})
	}
}

func TestDoHCertificateAndOfficialParser(t *testing.T) {
	workDir := t.TempDir()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "dns.example.com"},
		DNSNames:     []string{"dns.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, cert, cert, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDer, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(workDir, "cert.pem")
	keyPath := filepath.Join(workDir, "key.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer}), 0600); err != nil {
		t.Fatal(err)
	}
	cfg := Defaults()
	cfg.DoH = true
	cfg.Domain = "dns.example.com"
	yaml, err := Generate(cfg, certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(yaml, "type: http_server") || !strings.Contains(yaml, "/dns-query") {
		t.Fatal("DoH listener and path were not generated")
	}
	if !strings.Contains(yaml, `listen: ":8443"`) {
		t.Fatal("unexpected DoH port")
	}
	cfg.HTTP3 = true
	cfg.DoT = true
	cfg.DoQ = true
	yaml, err = Generate(cfg, certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, term := range []string{
		"type: http_server",
		"enable_http3: true",
		"type: tcp_server",
		"tag: dot_public",
		"tag: doq_public",
		"type: quic_server",
		`listen: ":853"`,
	} {
		if !strings.Contains(yaml, term) {
			t.Errorf("missing encrypted DNS listener field: %s", term)
		}
	}
	cfg.Domain = "wrong.example.com"
	if _, err := Generate(cfg, certPath, keyPath); err == nil {
		t.Fatal("mismatched certificate accepted")
	}
	// A DNS subdomain can use its own certificate without changing the
	// (potentially unrelated) certificate used by the panel.
	cfg.Domain = "dns.example.com"
	cfg.CertFile = certPath
	cfg.KeyFile = keyPath
	dedicated, err := Generate(cfg, "/nonexistent/panel/cert.pem", "/nonexistent/panel/key.pem")
	if err != nil {
		t.Fatalf("valid dedicated DNS certificate rejected: %v", err)
	}
	for _, path := range []string{certPath, keyPath} {
		if !strings.Contains(dedicated, strconv.Quote(path)) {
			t.Fatalf("OxiDNS YAML does not use dedicated certificate file %q", path)
		}
	}
	if err := ValidateConfiguration(cfg, "", ""); err != nil {
		t.Fatalf("preflight rejected DNS-specific certificate: %v", err)
	}

	cfg.KeyFile = ""
	if _, err := Generate(cfg, certPath, keyPath); err == nil || !strings.Contains(err.Error(), "оба пути") {
		t.Fatalf("half-configured dedicated certificate accepted: %v", err)
	}
	cfg.CertFile = "cert.pem"
	cfg.KeyFile = "key.pem"
	if _, err := Generate(cfg, certPath, keyPath); err == nil || !strings.Contains(err.Error(), "абсолютными") {
		t.Fatalf("relative certificate paths accepted: %v", err)
	}
	cfg.CertFile, cfg.KeyFile = certPath, keyPath
	cfg.Domain = "another.example.com"
	if err := ValidateConfiguration(cfg, "", ""); err == nil || !strings.Contains(err.Error(), "DNS-поддомену") {
		t.Fatalf("certificate for another DNS subdomain accepted: %v", err)
	}

	if binary := os.Getenv("OXIDNS_CHECK_BINARY"); binary != "" {
		configPath := filepath.Join(workDir, "config.yaml")
		if err := os.WriteFile(configPath, []byte(yaml), 0600); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "check", "-c", configPath, "-d", workDir)
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("official OxiDNS rejected DoH: %v\n%s\n%s", err, output, yaml)
		}
	}
}

// TestOfficialOxiDNSForwardsRealTraffic starts the actual official OxiDNS
// process with a deterministic local stub upstream. No external DNS service
// or firewall access to port 853 is needed.
func TestOfficialOxiDNSForwardsRealTraffic(t *testing.T) {
	binaryPath := os.Getenv("OXIDNS_CHECK_BINARY")
	if binaryPath == "" {
		t.Skip("OXIDNS_CHECK_BINARY is not set")
	}
	upstreamConn, err := net.ListenPacket("udp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer upstreamConn.Close()
	upstreamAddr := upstreamConn.LocalAddr().String()

	go func() {
		buffer := make([]byte, 2048)
		for {
			n, peer, err := upstreamConn.ReadFrom(buffer)
			if err != nil {
				return
			}
			if n < 17 {
				continue
			}
			// A client query can include EDNS0 OPT additional records.
			// Keep only the question, so the synthetic A answer follows it
			// directly rather than accidentally parsing OPT as an answer.
			questionEnd := 12
			valid := true
			for {
				if questionEnd >= n {
					valid = false
					break
				}
				labelLength := int(buffer[questionEnd])
				questionEnd++
				if labelLength == 0 {
					break
				}
				if labelLength > 63 || questionEnd+labelLength > n {
					valid = false
					break
				}
				questionEnd += labelLength
			}
			if !valid || questionEnd+4 > n {
				continue
			}
			questionEnd += 4
			packet := make([]byte, questionEnd)
			copy(packet, buffer[:questionEnd])
			// Reuse the DNS transaction ID and question, returning a valid
			// single IPv4 answer (203.0.113.7) for the queried name.
			packet[2], packet[3] = 0x81, 0x80
			binary.BigEndian.PutUint16(packet[6:8], 1)
			binary.BigEndian.PutUint16(packet[8:10], 0)
			binary.BigEndian.PutUint16(packet[10:12], 0)
			answer := []byte{0xc0, 0x0c, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x3c, 0x00, 0x04, 203, 0, 113, 7}
			packet = append(packet, answer...)
			_, _ = upstreamConn.WriteTo(packet, peer)
		}
	}()

	cfg := Defaults()
	config, err := Generate(cfg, "", "")
	if err != nil {
		t.Fatal(err)
	}
	original := "addr: \"tls://dns.quad9.net:853\""
	replacement := fmt.Sprintf("addr: \"udp://%s\"", upstreamAddr)
	if !strings.Contains(config, original) {
		t.Fatalf("missing upstream in generated YAML")
	}
	config = strings.Replace(config, original, replacement, 1)
	config = strings.Replace(config, "          bootstrap: \"9.9.9.9:53\"\n", "", 1)
	workDir := t.TempDir()
	configPath := filepath.Join(workDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	command := exec.Command(binaryPath, "start", "-c", configPath, "-d", workDir)
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = command.Process.Kill()
		_ = command.Wait()
	}()

	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		err = checkDNSAt(ctx, "127.0.0.1:15353")
		cancel()
		if err == nil {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("official OxiDNS failed to forward local query: %v\nservice log: %s", err, output.String())
}

func TestHasManagedServer(t *testing.T) {
	original := map[string]any{"servers": []any{
		map[string]any{"tag": "cloudflare", "type": "https", "server": "1.1.1.1"},
	}}
	if HasManagedServer(original) {
		t.Fatal("ordinary DNS was treated as an OxiDNS overlay")
	}
	withOverlay, err := WithSingBoxDNS(original)
	if err != nil {
		t.Fatal(err)
	}
	if !HasManagedServer(withOverlay) {
		t.Fatal("managed DNS overlay was not detected")
	}
	if !HasManagedServer(map[string]any{"servers": []any{
		map[string]any{"tag": Tag, "type": "udp", "server": "127.0.0.1"},
	}}) {
		t.Fatal("JSON-decoded managed DNS overlay was not detected")
	}
}

func TestWaitDoHReadyUsesConfiguredListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := waitDoHReady(ctx, port); err != nil {
		t.Fatalf("listening HTTPS port was not detected: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	cancelled, stop := context.WithCancel(context.Background())
	stop()
	if err := waitDoHReady(cancelled, port); err == nil {
		t.Fatal("closed DoH listener incorrectly accepted")
	}
}

func TestEncryptedDNSPortValidationAndCompatibility(t *testing.T) {
	cfg := Defaults()
	if cfg.DoTPort != 853 || cfg.DoQPort != 853 || cfg.Port != 8443 {
		t.Fatal("incorrect standard ports")
	}
	for _, test := range []struct {
		name string
		cfg  Settings
	}{
		{name: "H3 requires DoH", cfg: Settings{HTTP3: true, Upstream: "quad9"}},
		{name: "DoT conflicts local DNS", cfg: Settings{DoT: true, DoTPort: localPort, Upstream: "quad9"}},
		{name: "DoQ conflicts local DNS", cfg: Settings{DoQ: true, DoQPort: localPort, Upstream: "quad9"}},
		{name: "DoH and DoT TCP collision", cfg: Settings{DoH: true, DoT: true, Port: 8443, DoTPort: 8443, Upstream: "quad9"}},
		{name: "HTTP3 and DoQ UDP collision", cfg: Settings{DoH: true, HTTP3: true, DoQ: true, Port: 8443, DoQPort: 8443, Upstream: "quad9"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := Generate(test.cfg, "", ""); err == nil {
				t.Fatal("accepted invalid public encrypted DNS settings")
			}
		})
	}
	legacy := `{"enabled":false,"domain":"","doh":false,"port":8443,"upstream":"quad9","useSingBox":true}`
	var restored Settings
	if err := json.Unmarshal([]byte(legacy), &restored); err != nil {
		t.Fatal(err)
	}
	normalizePorts(&restored)
	if restored.DoTPort != 853 || restored.DoQPort != 853 || restored.DoT || restored.DoQ || restored.HTTP3 {
		t.Errorf("legacy settings did not preserve safe public DNS defaults: %+v", restored)
	}
}

func TestBinaryReadiness(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "binary")
	if binaryReady(path) || binaryReady(dir) {
		t.Fatal("missing file or directory accepted")
	}
	for _, tc := range []struct {
		content string
		mode    os.FileMode
		ready   bool
	}{
		{"", 0755, false}, {"binary", 0644, false}, {"binary", 0755, true},
	} {
		if err := os.WriteFile(path, []byte(tc.content), tc.mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, tc.mode); err != nil {
			t.Fatal(err)
		}
		if got := binaryReady(path); got != tc.ready {
			t.Fatalf("mode %o, contents %q: ready=%v", tc.mode, tc.content, got)
		}
	}
}

func TestSetupRequiresInstalledBinary(t *testing.T) {
	if binaryReady(binaryPath) {
		t.Skip("component already installed")
	}
	err := Setup(context.Background(), Defaults(), "", "")
	if err == nil || !strings.Contains(err.Error(), "сначала установите OxiDNS") {
		t.Fatalf("expected installation prerequisite, got %v", err)
	}
}
