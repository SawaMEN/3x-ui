package externalvpn

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
)

func TestFPTNCertificateUsesValidatedPrivateSnapshot(t *testing.T) {
	cert, key, err := newFPTNCertificate("localhost")
	if err != nil {
		t.Fatal(err)
	}
	source := t.TempDir()
	certPath, keyPath := filepath.Join(source, "cert.pem"), filepath.Join(source, "key.pem")
	if err := os.WriteFile(certPath, []byte(cert), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, []byte(key), 0o600); err != nil {
		t.Fatal(err)
	}
	target := t.TempDir()
	inst := Instance{Settings: Settings{Hostname: "localhost", Certificate: certPath, PrivateKey: keyPath}}
	gotCert, gotKey, err := fptnCertificateFiles(inst, target)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(gotCert) != target || filepath.Dir(gotKey) != target {
		t.Fatal("runtime reused user-provided certificate paths")
	}
	if err := os.WriteFile(certPath, []byte("changed after validation"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := tls.LoadX509KeyPair(gotCert, gotKey); err != nil {
		t.Fatalf("source mutation changed the validated snapshot: %v", err)
	}
	if info, err := os.Stat(gotKey); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("snapshot key must be private: %v", err)
	}
}
