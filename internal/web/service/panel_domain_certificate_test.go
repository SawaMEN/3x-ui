package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

func TestPanelDomainCertificateRequiresSavedLocalDirectDomainAndMatchingPair(t *testing.T) {
	cert, key := writeTestCertificate(t)
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: "golden-fixture.test", Mode: "direct", TLS: "auto", Enabled: true}}
	status := domainfront.DomainFrontStatus{Config: cfg, Running: true, Certificates: []domainfront.DomainFrontCertificate{{Name: "golden-fixture.test", CertFile: cert, KeyFile: key}}}
	now := time.Now()
	got, err := panelDomainCertificate(status, "golden-fixture.test", now)
	if err != nil || got.Domain != "golden-fixture.test" || got.CertFile != cert || got.KeyFile != key || got.NodeID != 0 || got.ExpiresAt <= now.Unix() {
		t.Fatalf("panel pair: %+v %v", got, err)
	}
	for _, mode := range []string{"cdn", "sni_only", "shared_sni", "sub_link_only", "cert_only"} {
		changed := status
		changed.Config.Domains = []domainfront.DomainFrontDomain{{Name: "golden-fixture.test", Mode: mode, TLS: "auto", Enabled: true}}
		if _, err := panelDomainCertificate(changed, "golden-fixture.test", now); err == nil {
			t.Errorf("accepted %s as panel direct source", mode)
		}
	}
	for _, name := range []string{"missing.example.org", "https://golden-fixture.test", "*.golden-fixture.test"} {
		if _, err := panelDomainCertificate(status, name, now); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	if _, err := panelDomainCertificate(status, "golden-fixture.test", now.Add(48*time.Hour)); err == nil {
		t.Fatal("accepted expired pair")
	}
	status.Config.Staging = true
	if _, err := panelDomainCertificate(status, "golden-fixture.test", now); err == nil {
		t.Fatal("accepted staging pair")
	}
	status.Config.Staging = false
	_, otherKey := writeTestCertificate(t)
	status.Certificates[0].KeyFile = otherKey
	if _, err := panelDomainCertificate(status, "golden-fixture.test", now); err == nil {
		t.Fatal("accepted mismatched private key")
	}
}

func TestPanelCertificateSharingKeepsGatewayIssuerAndPreventsSourceRemoval(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", root)
	cert, key := writeTestCertificate(t)
	cache := filepath.Join(root, "domain-tls", "certificates", "issuer", "golden-fixture.test")
	if err := os.MkdirAll(cache, 0700); err != nil {
		t.Fatal(err)
	}
	copyFile := func(source, target string) {
		t.Helper()
		data, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	managedCert, managedKey := filepath.Join(cache, "site.crt"), filepath.Join(cache, "site.key")
	copyFile(cert, managedCert)
	copyFile(key, managedKey)
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: "golden-fixture.test", Mode: "direct", TLS: "auto", Enabled: true}}
	settings := &entity.AllSetting{WebDomain: "golden-fixture.test", WebCertFile: managedCert, WebKeyFile: managedKey}
	files := inheritedDomainCertificates(cfg, settings, nil, false)
	if len(files) != 0 {
		t.Fatalf("gateway inherited its own panel copy and lost ACME renewal: %+v", files)
	}
	if err := validatePanelCertificateSource(cfg, cfg, settings.WebDomain, managedCert, managedKey); err != nil {
		t.Fatal(err)
	}
	disabled := cfg
	disabled.Enabled = false
	if err := validatePanelCertificateSource(cfg, disabled, settings.WebDomain, managedCert, managedKey); err == nil {
		t.Fatal("disabled the issuer of the panel certificate")
	}
	removed := cfg
	removed.Domains = nil
	if err := validatePanelCertificateSource(cfg, removed, settings.WebDomain, managedCert, managedKey); err == nil {
		t.Fatal("removed the issuer of the panel certificate")
	}
	if err := validatePanelCertificateSource(cfg, removed, "other.example.org", managedCert, managedKey); err != nil {
		t.Fatal("blocked removal after switching the panel domain")
	}
	if isDomainManagedCertificatePair(filepath.Join(root, "domain-tls-other", "a.crt"), managedKey) {
		t.Fatal("accepted unrelated directory")
	}
}
