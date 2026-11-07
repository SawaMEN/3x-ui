package domainfront

import (
	"strings"
	"testing"
)

func TestDomainRolesRejectUnsafeCombinations(t *testing.T) {
	base := func() DomainFrontConfig {
		cfg := DefaultConfig()
		cfg.Enabled = true
		cfg.AcceptTerms = true
		cfg.Domains = []DomainFrontDomain{{Name: "one.example.com", Mode: "direct", TLS: "auto", Enabled: true, InboundIDs: []int{1}}}
		return cfg
	}
	for _, tc := range []struct {
		name   string
		change func(*DomainFrontConfig)
		want   string
	}{
		{"sub-only panel", func(c *DomainFrontConfig) {
			c.Domains[0].Mode = "sub_link_only"
			c.Domains[0].InboundIDs = nil
			c.Domains[0].Panel = true
		}, "subscription-only domains cannot expose"},
		{"sub-only inbound", func(c *DomainFrontConfig) { c.Domains[0].Mode = "sub_link_only" }, "subscription-only domains cannot expose"},
		{"relay insecure origin", func(c *DomainFrontConfig) {
			c.Domains[0].Mode = "relay"
			c.Domains[0].Origin = "http://origin.example.com"
		}, "Relay requires an HTTPS origin"},
		{"relay cycle", func(c *DomainFrontConfig) {
			c.Domains[0].Mode = "relay"
			c.Domains[0].Origin = "https://two.example.com"
			c.Domains = append(c.Domains, DomainFrontDomain{Name: "two.example.com", Mode: "relay", TLS: "auto", Origin: "https://one.example.com"})
		}, "Relay origin cycle"},
		{"terms", func(c *DomainFrontConfig) { c.AcceptTerms = false }, "accept the Let's Encrypt terms"},
		{"canonical disabled", func(c *DomainFrontConfig) { c.Domains[0].Enabled = false; c.SubscriptionDomain = c.Domains[0].Name }, "canonical subscription domain must be an enabled domain"},
		{"duplicate", func(c *DomainFrontConfig) { c.Domains = append(c.Domains, c.Domains[0]) }, "duplicate domain"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base()
			tc.change(&cfg)
			err := cfg.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
