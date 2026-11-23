package domainfront

import (
	"strings"
	"testing"
)

func TestSNIOnlyCustomSiteHTMLValidation(t *testing.T) {
	html := "<!doctype html><html><head></head><body>Hello</body></html>"
	cfg := DefaultConfig()
	cfg.Domains = []DomainFrontDomain{{
		Name: "cover.example.org", Enabled: true, Mode: "sni_only", TLS: "auto",
		SiteCatalog: "custom", SiteTemplate: "html", SiteHTML: html,
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid custom site rejected: %v", err)
	}
	for _, bad := range []struct{ catalog, template, html string }{
		{"custom", "html", ""},
		{"custom", "html", "<div>fragment</div>"},
		{"custom", "html", "<html>\x00</html>"},
		{"custom", "html", "<html>" + strings.Repeat("x", CoverSiteHTMLMaxBytes)},
		{"builtin", "notes", html},
	} {
		cfg.Domains[0].SiteCatalog, cfg.Domains[0].SiteTemplate, cfg.Domains[0].SiteHTML = bad.catalog, bad.template, bad.html
		if err := cfg.Validate(); err == nil {
			t.Fatalf("invalid custom page accepted: %+v", bad)
		}
	}
}

func TestCoverSiteWithExistingDomainRoles(t *testing.T) {
	for _, mode := range []string{"direct", "cdn", "relay", "sub_link_only", "shared_sni", "cert_only"} {
		t.Run(mode, func(t *testing.T) {
			cfg := DefaultConfig()
			d := DomainFrontDomain{Name: "site.example.org", Mode: mode, Enabled: true, TLS: "auto",
				SiteCatalog: "builtin", SiteTemplate: "notes"}
			if mode == "relay" {
				d.Origin = "https://origin.example.org"
			}
			if mode == "cert_only" {
				d.TLS = "certmagic"
			}
			cfg.Domains = []DomainFrontDomain{d}
			allowed := mode != "shared_sni" && mode != "cert_only"
			if err := cfg.Validate(); (err == nil) != allowed {
				t.Fatalf("mode %s: unexpected validation result: %v", mode, err)
			}
			if d.HasCoverSite() != allowed {
				t.Fatalf("mode %s: unexpected website state", mode)
			}
		})
	}
}

func TestCoverSiteHTMLSizeAndConcreteHostname(t *testing.T) {
	cfg := DefaultConfig()
	const tags = "<html></html>"
	cfg.Domains = []DomainFrontDomain{{
		Name: "site.example.org", Mode: "direct", Enabled: true, TLS: "auto",
		SiteCatalog: "custom", SiteTemplate: "html",
		SiteHTML: "<html>" + strings.Repeat("x", CoverSiteHTMLMaxBytes-len(tags)) + "</html>",
	}}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("maximum size HTML rejected: %v", err)
	}
	cfg.Domains[0].SiteHTML += "x"
	if err := cfg.Validate(); err == nil {
		t.Fatal("oversized HTML accepted")
	}
	cfg.Domains[0].SiteHTML = "<html>test</html>"
	cfg.Domains[0].Name = "*.example.org"
	cfg.DNS01Provider, cfg.DNS01Zone = "digitalocean", "example.org"
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "concrete hostname") {
		t.Fatalf("wildcard website accepted: %v", err)
	}
}
