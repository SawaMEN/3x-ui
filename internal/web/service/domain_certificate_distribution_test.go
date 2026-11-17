package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"
)

func testDistributionPair(t *testing.T, host string, expires time.Time) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     expires,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, &x509.Certificate{
		SerialNumber: big.NewInt(1),
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     expires,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	privateDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privateDER})
}

func TestCertificateDistributionHostnameValidation(t *testing.T) {
	name, err := distributionDomain("EXAMPLE.COM.")
	if err != nil || name != "example.com" {
		t.Fatalf("normalized hostname: %q %v", name, err)
	}
	for _, invalid := range []string{
		"", "*.example.com", "https://example.com", "../secret",
		"//example.com", "example.com:443", "example.com/path",
		"https://user:password@example.com:443/path?query=value#fragment",
	} {
		if name, err := distributionDomain(invalid); err == nil {
			t.Errorf("accepted invalid hostname %q as %q", invalid, name)
		}
	}
}

func TestCertificateDistributionPairValidation(t *testing.T) {
	cert, key := testDistributionPair(t, "owned.example.com", time.Now().Add(time.Hour))
	if _, err := validateDistributedPair("other.example.com", cert, key); err == nil {
		t.Fatal("accepted a certificate for the wrong hostname")
	}
	if _, err := validateDistributedPair("owned.example.com", cert, []byte("invalid")); err == nil {
		t.Fatal("accepted a mismatched private key")
	}
	if _, err := validateDistributedPair("owned.example.com", []byte(strings.Repeat("X", maxDistributedPEMBytes+1)), key); err == nil {
		t.Fatal("accepted an oversized certificate")
	}
	expired, expiredKey := testDistributionPair(t, "owned.example.com", time.Now().Add(-time.Minute))
	if _, err := validateDistributedPair("owned.example.com", expired, expiredKey); err == nil {
		t.Fatal("accepted an expired certificate")
	}
	// A self-signed server certificate must not be distributed as ordinary
	// publicly trusted TLS even when its hostname and key match.
	if _, err := validateDistributedPair("owned.example.com", cert, key); err == nil {
		t.Fatal("accepted an untrusted self-signed certificate")
	}
}
