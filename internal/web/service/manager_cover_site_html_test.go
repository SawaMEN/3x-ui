package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

func TestManagedCoverSiteCustomHTML(t *testing.T) {
	html := "<!doctype html><html><head><title>Example</title></head><body>Owner's site</body></html>"
	site, err := managedCoverSiteOptions("custom", "html", html)
	if err != nil || site == nil || site.Mode != "html" {
		t.Fatalf("custom page not accepted: %+v, %v", site, err)
	}
	dir := t.TempDir()
	path, err := prepareTelemtWebSite("example.org", filepath.Join(dir, "default"), filepath.Join(dir, "snapshots"), site)
	if err != nil || path == "" {
		t.Fatalf("custom page cannot be published: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(path, "index.html"))
	if err != nil || string(data) != html {
		t.Fatalf("custom HTML changed during publish: %v", err)
	}
	for _, bad := range []struct{ catalog, template, html string }{
		{"custom", "html", ""},
		{"builtin", "coffee", html},
		{"custom", "unknown", html},
		{"custom", "html", "<p>fragment</p>"},
		{"custom", "html", "<html>" + strings.Repeat("a", telemtWebSiteMaxBytes)},
	} {
		if _, err := managedCoverSiteOptions(bad.catalog, bad.template, bad.html); err == nil {
			t.Fatalf("invalid custom page accepted: %+v", bad)
		}
	}
	cfg := domainfront.DefaultConfig()
	out, err := managerCoverSiteConfig(cfg, ManagerCoverSiteInput{
		Domain: "example.org", Email: "owner@example.org", AcceptTerms: true,
		SiteCatalog: "custom", SiteTemplate: "html", SiteHTML: html,
	})
	if err != nil || len(out.Domains) != 1 || out.Domains[0].SiteHTML != html {
		t.Fatalf("custom HTML not persisted in gateway config: %+v %v", out.Domains, err)
	}
	out, err = managerCoverSiteConfig(out, ManagerCoverSiteInput{
		Domain: "example.org", Email: "owner@example.org", AcceptTerms: true,
		SiteCatalog: "builtin", SiteTemplate: "notes",
	})
	if err != nil || out.Domains[0].SiteHTML != "" {
		t.Fatalf("switching back retained HTML: %+v %v", out.Domains, err)
	}
}
