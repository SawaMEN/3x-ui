package domainfront

import (
	"strings"
	"testing"
)

func TestWildcardDNSRequiresDNS01AndMatchingZone(t *testing.T) {
	base := func() DomainFrontConfig {
		c := DefaultConfig()
		c.Enabled = true
		c.AcceptTerms = true
		c.Domains = []DomainFrontDomain{{Name: "*.sub.example.org", Enabled: true, Mode: "direct", TLS: "auto"}}
		return c
	}
	cfg := base()
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("wildcard without DNS provider accepted: %v", err)
	}
	cfg = base()
	cfg.DNS01Provider = "cloudflare"
	cfg.DNS01Zone = "example.org"
	cfg.DNS01ZoneID = strings.Repeat("a", 32)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("DNS-01 wildcard should be accepted: %v", err)
	}
	cfg.Domains[0].Name = "*.example.net"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "zone does not own") {
		t.Fatalf("wildcard in another zone was accepted: %v", err)
	}
	cfg = base()
	cfg.DNS01Provider = "cloudflare"
	cfg.DNS01Zone = "example.org"
	cfg.DNS01ZoneID = strings.Repeat("a", 32)
	cfg.Domains[0].TLS = "manual"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "wildcard") {
		t.Fatalf("wildcard manual certificate should be rejected: %v", err)
	}
}

func TestWildcardOnlyMatchesOneLabelAndExactDomainWins(t *testing.T) {
	r := &frontRuntime{cfg: DomainFrontConfig{Domains: []DomainFrontDomain{
		{Name: "*.example.org", Enabled: true, Mode: "direct"},
		{Name: "api.example.org", Enabled: true, Mode: "sub_link_only"},
	}}}
	for _, test := range []struct {
		host string
		matched string
	}{
		{"api.example.org", "api.example.org"},
		{"shop.example.org", "*.example.org"},
		{"deep.shop.example.org", ""},
		{"example.org", ""},
		{"shop.example.net", ""},
		{"shop.example.org:443", "*.example.org"},
	} {
		d, ok := r.domain(test.host)
		if test.matched == "" {
			if ok {
				t.Errorf("%q unexpectedly matched %q", test.host, d.Name)
			}
		} else if !ok || d.Name != test.matched {
			t.Errorf("%q matched %q (ok=%v), want %q", test.host, d.Name, ok, test.matched)
		}
	}
}
