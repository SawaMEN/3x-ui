package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTelemtWebNginxVhostCollision(t *testing.T) {
	config := `# configuration file /etc/nginx/conf.d/3x-ui-telemt-web.conf:
server {
    listen 443 ssl;
    server_name web.example.com;
    location / { proxy_pass http://127.0.0.1:15080; }
}
# configuration file /etc/nginx/conf.d/old-website.conf:
server {
    listen 80;
    server_name acme.example.com;
}
server {
    listen [::]:443 ssl;
    server_name existing.example.com *.shared.example.com;
    location / { return 200; }
}
# configuration file /etc/nginx/conf.d/other.conf:
server {
    listen 8443 ssl;
    server_name other.example.com;
}
`
	for _, tt := range []struct {
		domain string
		want bool
	}{
		{"web.example.com", false}, // our own managed WEB vhost
		{"acme.example.com", false}, // HTTP-only ACME
		{"existing.example.com", true},
		{"a.shared.example.com", true},
		{"other.example.com", false}, // only 8443
		{"missing.example.com", false},
	} {
		if got := telemtWebNginxConfigHasHost(config, tt.domain); got != tt.want {
			t.Errorf("nginx vhost conflict for %s = %v, want %v", tt.domain, got, tt.want)
		}
	}
}

func telemtWebTestCertificate(t *testing.T, key *ecdsa.PrivateKey, notAfter time.Time) (string, string) {
	t.Helper()
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<60))
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{CommonName: "web.example.com"},
		DNSNames: []string{"web.example.com"},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter: notAfter,
		KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certFile, keyFile := filepath.Join(dir,"cert.pem"), filepath.Join(dir,"key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type:"CERTIFICATE", Bytes:der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type:"EC PRIVATE KEY", Bytes:privateDER}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certFile, keyFile
}

func TestTelemtWebCertificatePairValidation(t *testing.T) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	cert, private := telemtWebTestCertificate(t, key, time.Now().Add(time.Hour))
	if !telemtWebCertificatePairReady(cert, private, "web.example.com") {
		t.Fatal("valid domain certificate and key should be accepted")
	}
	if telemtWebCertificatePairReady(cert, private, "elsewhere.example.com") {
		t.Fatal("wrong hostname must be rejected")
	}
	anotherKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	_, mismatch := telemtWebTestCertificate(t, anotherKey, time.Now().Add(time.Hour))
	if telemtWebCertificatePairReady(cert, mismatch, "web.example.com") {
		t.Fatal("mismatched private key must be rejected")
	}
	expiredCert, expiredKey := telemtWebTestCertificate(t, key, time.Now().Add(-time.Minute))
	if telemtWebCertificatePairReady(expiredCert, expiredKey, "web.example.com") {
		t.Fatal("expired certificate must be rejected")
	}
}
