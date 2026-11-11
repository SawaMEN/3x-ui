package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

func TestManagerCoverSiteConfigUsesExistingCertMagic(t *testing.T) {
	cfg := domainfront.DefaultConfig()
	cfg.Domains = []domainfront.DomainFrontDomain{{
		Name: "sub.example.org", Mode: "sub_link_only",
		Enabled: true, TLS: "auto", InboundIDs: []int{},
	}}
	out, err := managerCoverSiteConfig(cfg, ManagerCoverSiteInput{
		NodeID: 0, Domain: "DECOY.EXAMPLE.ORG.", Email: "owner@example.org", AcceptTerms: true,
		SiteCatalog: "builtin", SiteTemplate: "coffee",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Enabled || !out.AcceptTerms || out.Email != "owner@example.org" ||
		len(out.Domains) != 2 || out.Domains[0].Name != "sub.example.org" ||
		out.Domains[1].Name != "decoy.example.org" ||
		out.Domains[1].Mode != "sni_only" ||
		out.Domains[1].TLS != "certmagic" ||
		out.Domains[1].SiteCatalog != "builtin" || out.Domains[1].SiteTemplate != "coffee" ||
		!out.Domains[1].Enabled || out.Domains[1].Panel {
		t.Fatalf("unsafe cover configuration: %+v", out)
	}
	if len(cfg.Domains) != 1 || cfg.Enabled {
		t.Fatal("builder mutated original config")
	}
	again, err := managerCoverSiteConfig(out, ManagerCoverSiteInput{
		Domain: "decoy.example.org", Email: "owner@example.org", AcceptTerms: true,
		SiteCatalog: "mtproxyl", SiteTemplate: "catrunner",
	})
	if err != nil || len(again.Domains) != 2 || again.Domains[1].SiteTemplate != "catrunner" {
		t.Fatalf("same cover domain was duplicated: %+v, %v", again, err)
	}
}

func TestManagerCoverSiteConfigRefusesUnsafeOrConflictingDomain(t *testing.T) {
	req := ManagerCoverSiteInput{
		Domain: "site.example.org", Email: "owner@example.org", AcceptTerms: true,
	}
	cfg := domainfront.DefaultConfig()
	cfg.Domains = []domainfront.DomainFrontDomain{{
		Name: req.Domain, Mode: "direct", TLS: "manual", Enabled: true,
		CertFile: "/custom.crt", KeyFile: "/custom.key",
	}}
	if _, err := managerCoverSiteConfig(cfg, req); err == nil {
		t.Fatal("an existing real website was overwritten")
	}
	cfg = domainfront.DefaultConfig()
	for _, input := range []ManagerCoverSiteInput{
		{Domain: req.Domain, Email: req.Email},
		{Domain: "https://site.example.org/path", Email: req.Email, AcceptTerms: true},
		{Domain: "127.0.0.1", Email: req.Email, AcceptTerms: true},
		{Domain: req.Domain, Email: "not-an-email", AcceptTerms: true},
		{Domain: req.Domain, Email: req.Email, AcceptTerms: true, SiteCatalog: "builtin", SiteTemplate: "unknown"},
		{Domain: req.Domain, Email: req.Email, AcceptTerms: true, SiteCatalog: "mtproxyl"},
	} {
		if _, err := managerCoverSiteConfig(cfg, input); err == nil {
			t.Errorf("invalid domain provisioning was accepted: %+v", input)
		}
	}
	if _, err := managerCoverSiteConfig(domainfront.DomainFrontConfig{}, req); err == nil {
		t.Fatal("missing listener config was reset instead of rejected")
	}
}

func TestManagerCoverSiteConfigRetainsPreexistingManagedWebsite(t *testing.T) {
	cfg := domainfront.DefaultConfig()
	cfg.Domains = []domainfront.DomainFrontDomain{{
		Name: "site.example.org", Mode: "sni_only", TLS: "certmagic", Enabled: false,
	}}
	out, err := managerCoverSiteConfig(cfg, ManagerCoverSiteInput{
		Domain: "site.example.org", Email: "owner@example.org", AcceptTerms: true,
	})
	if err != nil || len(out.Domains) != 1 || !out.Domains[0].Enabled {
		t.Fatalf("existing decoy was not activated: %+v, %v", out, err)
	}
}
