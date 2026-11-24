package service

import (
	"encoding/json"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
)

func TestSubscriptionPublicAddressAcrossFormats(t *testing.T) {
	setupConflictDB(t)
	s := &SettingService{}
	for key, value := range map[string]string{
		"subPublicURL": "https://cdn.example.com", "subDomain": "origin.example.com", "subAllowedDomains": "extra.example.com",
		"subPort": "2096", "subPath": "/sub/", "subJsonPath": "/json/", "subClashPath": "/clash/",
		"subURI": "https://old.example.com/sub/", "subJsonURI": "", "subClashURI": "",
	} {
		if err := s.saveSetting(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.BuildSubURIBase("panel.example.com"); got != "https://cdn.example.com" {
		t.Fatalf("base = %q", got)
	}
	for _, test := range []struct {
		get  func() (string, error)
		want string
	}{
		{s.GetSubURI, "https://cdn.example.com/sub/"},
		{s.GetSubJsonURI, "https://cdn.example.com/json/"},
		{s.GetSubClashURI, "https://cdn.example.com/clash/"},
	} {
		got, err := test.get()
		if err != nil || got != test.want {
			t.Fatalf("URL = %q, %v, want %q", got, err, test.want)
		}
	}
	hosts, err := s.GetSubscriptionAllowedHosts("origin.example.com")
	if err != nil || len(hosts) != 3 || !hosts["cdn.example.com"] || !hosts["extra.example.com"] || !hosts["origin.example.com"] || hosts["panel.example.com"] {
		t.Fatalf("hosts = %#v, %v", hosts, err)
	}
	if err := s.saveSetting("subPublicURL", ""); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetSubURI(); err != nil || got != "https://old.example.com/sub/" {
		t.Fatalf("legacy URL = %q, %v", got, err)
	}
}

func TestSubscriptionDomainValidation(t *testing.T) {
	for _, tc := range []struct {
		name     string
		settings entity.AllSetting
		want     string
	}{
		{"path", entity.AllSetting{SubPublicURL: "https://sub.example.com/private/"}, "subscription public URL must be an HTTP/HTTPS origin without a path, credentials, query or fragment"},
		{"empty query", entity.AllSetting{SubPublicURL: "https://sub.example.com?"}, "subscription public URL must be an HTTP/HTTPS origin without a path, credentials, query or fragment"},
		{"empty fragment", entity.AllSetting{SubPublicURL: "https://sub.example.com#"}, "subscription public URL must be an HTTP/HTTPS origin without a path, credentials, query or fragment"},
		{"empty port", entity.AllSetting{SubPublicURL: "https://sub.example.com:"}, "subscription public port must be between 1 and 65535"},
		{"zero port", entity.AllSetting{SubPublicURL: "https://sub.example.com:0"}, "subscription public port must be between 1 and 65535"},
		{"large port", entity.AllSetting{SubPublicURL: "https://sub.example.com:65536"}, "subscription public port must be between 1 and 65535"},
		{"cdn without address", entity.AllSetting{SubCDNEnable: true}, "CDN mode requires a subscription public URL"},
		{"cdn without TLS", entity.AllSetting{SubCDNEnable: true, SubPublicURL: "http://sub.example.com"}, "CDN subscription public URL must use HTTPS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSubscriptionDomains(&tc.settings, false)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestManagedSubscriptionOriginOverridesSavedPublicAddress(t *testing.T) {
	setupConflictDB(t)
	s := &SettingService{}
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.SubscriptionDomain = "managed.example.com"
	cfg.PublicPort = 8443
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: cfg.SubscriptionDomain, Enabled: true, Mode: "cdn", TLS: "auto"}}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"domainFrontConfig": string(raw), "subPublicURL": "https://external.example.com",
		"subCDNEnable": "false", "subPath": "/sub/", "subJsonPath": "/json/", "subClashPath": "/clash/",
	} {
		if err := s.setString(key, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		get  func() (string, error)
		want string
	}{
		{s.GetSubURI, "https://managed.example.com:8443/sub/"},
		{s.GetSubJsonURI, "https://managed.example.com:8443/json/"},
		{s.GetSubClashURI, "https://managed.example.com:8443/clash/"},
	} {
		if got, err := tc.get(); err != nil || got != tc.want {
			t.Fatalf("origin = %q, %v; want %q", got, err, tc.want)
		}
	}
	if cdn, err := s.GetSubCDNEnable(); err != nil || !cdn {
		t.Fatalf("managed CDN = %v, %v", cdn, err)
	}
	cfg.Enabled = false
	raw, err = json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.setString("domainFrontConfig", string(raw)); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetSubPublicURL(); err != nil || got != "https://external.example.com" {
		t.Fatalf("fallback origin = %q, %v", got, err)
	}
	if cdn, err := s.GetSubCDNEnable(); err != nil || cdn {
		t.Fatalf("fallback CDN = %v, %v", cdn, err)
	}
}

func TestRestrictedSubscriptionHostsExcludeImplicitDomains(t *testing.T) {
	setupConflictDB(t)
	s := &SettingService{}
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.SubscriptionDomain = "managed.example.com"
	cfg.Domains = []domainfront.DomainFrontDomain{
		{Name: "managed.example.com", Enabled: true, Mode: "direct", TLS: "auto"},
		{Name: "panel.example.com", Enabled: true, Mode: "direct", TLS: "auto", Panel: true},
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{
		"domainFrontConfig": string(raw), "subRestrictDomains": "true",
		"subPublicURL": "https://selected.example.com:8443", "subAllowedDomains": "extra.example.com",
	} {
		if err := s.saveSetting(key, value); err != nil {
			t.Fatal(err)
		}
	}
	hosts, err := s.GetSubscriptionAllowedHosts("origin.example.com")
	if err != nil || len(hosts) != 2 || !hosts["managed.example.com"] || !hosts["extra.example.com"] {
		t.Fatalf("restricted hosts = %#v, %v", hosts, err)
	}
	if got, err := s.GetSubPublicURL(); err != nil || got != "https://managed.example.com" {
		t.Fatalf("public URL = %q, %v", got, err)
	}
	if err := s.saveSetting("subAllowedDomains", ""); err != nil {
		t.Fatal(err)
	}
	hosts, err = s.GetSubscriptionAllowedHosts("origin.example.com")
	if err != nil || len(hosts) != 1 || !hosts["managed.example.com"] {
		t.Fatalf("single domain = %#v, %v", hosts, err)
	}
	if err := validateSubscriptionDomains(&entity.AllSetting{SubRestrictDomains: true}, false); err == nil {
		t.Fatal("restricted access accepted without a public origin")
	}
	if err := s.saveSetting("subRestrictDomains", "false"); err != nil {
		t.Fatal(err)
	}
	hosts, err = s.GetSubscriptionAllowedHosts("origin.example.com")
	if err != nil || !hosts["managed.example.com"] || !hosts["panel.example.com"] || !hosts["origin.example.com"] {
		t.Fatalf("legacy hosts = %#v, %v", hosts, err)
	}
}

func TestRestrictedSubscriptionUsesManagedDomainAfterSettingsSave(t *testing.T) {
	setupConflictDB(t)
	s := &SettingService{}
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.SubscriptionDomain = "subscription.example.com"
	cfg.PublicPort = 8443
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: cfg.SubscriptionDomain, Enabled: true, Mode: "cdn", TLS: "auto"}}
	saveGateway := func() {
		t.Helper()
		raw, err := json.Marshal(cfg)
		if err != nil {
			t.Fatal(err)
		}
		if err := s.setString("domainFrontConfig", string(raw)); err != nil {
			t.Fatal(err)
		}
	}
	saveGateway()
	settings, err := s.GetAllSetting()
	if err != nil {
		t.Fatal(err)
	}
	settings.SubRestrictDomains = true
	settings.SubAllowedDomains = "alias.example.com"
	if err := s.UpdateAllSetting(settings, SecretClears{}); err != nil {
		t.Fatal(err)
	}
	if err := s.setString("subPublicURL", "https://subscription.example.com:8443"); err != nil {
		t.Fatal(err)
	}
	cfg.SubscriptionDomain, cfg.Domains[0].Name, cfg.PublicPort = "new.example.com", "new.example.com", 9443
	saveGateway()
	if got := s.BuildSubURIBase("panel.example.com"); got != "https://new.example.com:9443" {
		t.Fatalf("public origin = %q", got)
	}
	hosts, err := s.GetSubscriptionAllowedHosts("origin.example.com")
	if err != nil || len(hosts) != 2 || !hosts["new.example.com"] || !hosts["alias.example.com"] {
		t.Fatalf("restricted hosts = %#v, %v", hosts, err)
	}
}

func TestManagedSubscriptionUsesConfiguredListenerPort(t *testing.T) {
	setupConflictDB(t)
	s := &SettingService{}
	const name = "subscriptions.example.com"
	cfg := domainfront.DefaultConfig()
	cfg.Enabled, cfg.SubscriptionDomain = true, name
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: name, Enabled: true, Mode: "sub_link_only", TLS: "auto"}}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]string{"domainFrontConfig": string(raw), "subDomain": name, "subPort": "8443"} {
		if err := s.setString(key, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		override, listen, wantURL, wantDomain string
	}{
		{"", "", "https://" + name + ":8443", name},
		{"https://" + name + ":8443", "", "https://" + name + ":8443", name},
		{"https://" + name + ":9443", "", "https://" + name + ":8443", name},
		{"https://previous.example.com:443", "", "https://" + name + ":8443", name},
		{"", "127.0.0.1", "https://" + name, ""},
	} {
		t.Run(tc.override+tc.listen, func(t *testing.T) {
			if err := s.setString("subPublicURL", tc.override); err != nil {
				t.Fatal(err)
			}
			if err := s.setString("subListen", tc.listen); err != nil {
				t.Fatal(err)
			}
			if got, err := s.GetSubPublicURL(); err != nil || got != tc.wantURL {
				t.Fatalf("public subscription URL = %q, %v, want %q", got, err, tc.wantURL)
			}
			if got, err := s.ManagedSubscriptionListenerDomain(); err != nil || got != tc.wantDomain {
				t.Fatalf("subscription listener identity = %q, %v, want %q", got, err, tc.wantDomain)
			}
		})
	}
}

func TestDirectSubscriptionOriginRejectsInvalidListener(t *testing.T) {
	for _, settings := range []entity.AllSetting{
		{SubEnable: true, SubDomain: "sub.example.com", SubPort: 65536},
		{SubEnable: true, SubDomain: "sub.example.com", SubPort: 443, SubCertFile: "cert.pem"},
		{SubEnable: true, SubDomain: "sub.example.com", SubPort: 2096, SubListen: " 127.0.0.1 "},
	} {
		if got := directSubscriptionOrigin(&settings); got != "" {
			t.Fatalf("invalid listener published as %q", got)
		}
	}
}

func TestSubscriptionDomainValidationDeduplicatesAliases(t *testing.T) {
	settings := entity.AllSetting{SubAllowedDomains: "Alias.example.com, alias.example.com.; second.example.com"}
	if err := validateSubscriptionDomains(&settings, false); err != nil {
		t.Fatal(err)
	}
	if settings.SubAllowedDomains != "alias.example.com,second.example.com" {
		t.Fatalf("unexpected normalized aliases: %q", settings.SubAllowedDomains)
	}
}
