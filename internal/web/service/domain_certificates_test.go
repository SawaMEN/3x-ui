package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

func TestDomainCertificateReusesInstallerAndPreservesRenewal(t *testing.T) {
	cert, key := writeTestCertificate(t)
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: "golden-fixture.test", Enabled: true, TLS: "auto", Mode: "direct"}}
	settings := &entity.AllSetting{WebCertFile: cert, WebKeyFile: key}
	home := t.TempDir()
	dir := filepath.Join(home, "golden-fixture.test_ecc")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "golden-fixture.test.conf")
	original := "Le_Domain='golden-fixture.test'\nLe_Alt='alt.example.com'\nLe_Webroot='no'\nLe_RealFullChainPath='" + cert + "'\nLe_RealKeyPath='" + key + "'\nLe_ReloadCmd='do-not-change'\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\nprintf '%s\\n' '17 2 * * * \"" + home + "\"/acme.sh --cron --home \"" + home + "\" > /dev/null'\n"
	if err := os.WriteFile(filepath.Join(bin, "crontab"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	states := domainACMEStates([]string{home})
	files := inheritedDomainCertificates(cfg, settings, states, acmeScheduled([]string{home}))
	if got := files[cfg.Domains[0].Name]; got.Source != "panel" || got.Renewal != "acme.sh" || !got.AutoRenew || got.CertFile != cert {
		t.Fatalf("inherited = %#v", got)
	}
	if err := cfg.ValidateWithCertificates(files); err != nil {
		t.Fatalf("reusing a certificate must not require new issuance terms: %v", err)
	}
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	roots, rollback, err := prepareDomainACME(cfg, files, states)
	if err != nil {
		t.Fatal(err)
	}
	state, err := readDomainACME(path)
	if err != nil {
		t.Fatal(err)
	}
	if state.values["Le_Webroot"] != roots["golden-fixture.test"] || roots["alt.example.com"] == "" || state.values["Le_ReloadCmd"] != "do-not-change" {
		t.Fatalf("renewal state = %#v, roots = %#v", state.values, roots)
	}
	rollback()
	if raw, _ := os.ReadFile(path); string(raw) != original {
		t.Fatalf("rollback changed the existing ACME configuration: %s", raw)
	}
	_, _, err = prepareDomainACME(cfg, files, states)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Enabled = false
	_, _, err = prepareDomainACME(cfg, files, domainACMEStates([]string{home}))
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != original {
		t.Fatalf("disabling did not restore standalone: %s", raw)
	}
	if raw, _ := os.ReadFile(path + ".xui-webroot.json"); len(raw) != 0 {
		t.Fatal("left stale migration ownership")
	}
}

func TestDomainCertificateDNSRenewalAndUnrelatedDomainStayIndependent(t *testing.T) {
	cert, key := writeTestCertificate(t)
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: "other.example.com", Enabled: true, TLS: "auto", Mode: "direct"}}
	state := domainACME{path: filepath.Join(t.TempDir(), "cert.conf"), values: map[string]string{"Le_Webroot": "dns_cf", "Le_RealFullChainPath": cert, "Le_RealKeyPath": key}}
	files := inheritedDomainCertificates(cfg, &entity.AllSetting{WebCertFile: cert, WebKeyFile: key}, []domainACME{state}, true)
	if len(files) != 0 {
		t.Fatalf("wrong-domain certificate reused: %#v", files)
	}
	cfg.Domains[0].Name = "golden-fixture.test"
	files = inheritedDomainCertificates(cfg, &entity.AllSetting{WebCertFile: cert, WebKeyFile: key}, []domainACME{state}, true)
	if err := os.WriteFile(state.path, []byte("Le_Webroot='dns_cf'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := prepareDomainACME(cfg, files, []domainACME{state})
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(state.path); string(raw) != "Le_Webroot='dns_cf'\n" {
		t.Fatalf("DNS renewal modified: %s", raw)
	}
}

func TestDomainSettingsSeedExistingListenerAndPublicAddress(t *testing.T) {
	setupConflictDB(t)
	cert, key := writeTestCertificate(t)
	s := &SettingService{}
	for name, value := range map[string]string{"webCertFile": cert, "webKeyFile": key, "subPublicURL": "https://sub.example.com:8443", "subCDNEnable": "true"} {
		if err := s.setString(name, value); err != nil {
			t.Fatal(err)
		}
	}
	cfg, err := s.GetDomainFrontConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Enabled || cfg.PublicPort != 8443 || cfg.SubscriptionDomain != "sub.example.com" || len(cfg.Domains) != 2 || !cfg.Domains[0].Panel || cfg.Domains[1].Mode != "cdn" {
		t.Fatalf("seed = %#v", cfg)
	}
	status, err := DomainFrontStatus()
	if err != nil {
		t.Fatal(err)
	}
	if len(status.ExistingCertificates) != 1 || status.ExistingCertificates[0].Source != "panel" || status.ExistingCertificates[0].DaysLeft != 1 {
		t.Fatalf("existing certificates = %#v", status.ExistingCertificates)
	}
	raw, _ := json.Marshal(status)
	if strings.Contains(string(raw), "PRIVATE KEY") {
		t.Fatal("private key returned in status")
	}
}
