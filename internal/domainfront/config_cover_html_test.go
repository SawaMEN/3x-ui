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
		{"custom", "html", "<html>" + strings.Repeat("x", 256*1024)},
		{"builtin", "notes", html},
	} {
		cfg.Domains[0].SiteCatalog, cfg.Domains[0].SiteTemplate, cfg.Domains[0].SiteHTML = bad.catalog, bad.template, bad.html
		if err := cfg.Validate(); err == nil {
			t.Fatalf("invalid custom page accepted: %+v", bad)
		}
	}
}
