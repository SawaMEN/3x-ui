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
		{"cdn without address", entity.AllSetting{SubCDNEnable: true}, "CDN mode requires a subscription public URL"},
		{"cdn without TLS", entity.AllSetting{SubCDNEnable: true, SubPublicURL: "http://sub.example.com"}, "CDN subscription public URL must use HTTPS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateSubscriptionDomains(&tc.settings)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestExplicitSubscriptionOriginOverridesManagedDomain(t *testing.T) {
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
		{s.GetSubURI, "https://external.example.com/sub/"},
		{s.GetSubJsonURI, "https://external.example.com/json/"},
		{s.GetSubClashURI, "https://external.example.com/clash/"},
	} {
		if got, err := tc.get(); err != nil || got != tc.want {
			t.Fatalf("origin = %q, %v; want %q", got, err, tc.want)
		}
	}
	if cdn, err := s.GetSubCDNEnable(); err != nil || cdn {
		t.Fatalf("explicit CDN = %v, %v", cdn, err)
	}
	if err := s.setString("subPublicURL", ""); err != nil {
		t.Fatal(err)
	}
	if got, err := s.GetSubPublicURL(); err != nil || got != "https://managed.example.com:8443" {
		t.Fatalf("automatic origin = %q, %v", got, err)
	}
	if cdn, err := s.GetSubCDNEnable(); err != nil || !cdn {
		t.Fatalf("automatic CDN = %v, %v", cdn, err)
	}
}
