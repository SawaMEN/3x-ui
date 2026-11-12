package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

func TestManagedCoverSiteSummariesFilterAndFallback(t *testing.T) {
	cfg := domainfront.DomainFrontConfig{Domains: []domainfront.DomainFrontDomain{
		{Name: "notes.example.com", Mode: "sni_only", Enabled: true, SiteCatalog: "builtin", SiteTemplate: "notes"},
		{Name: "legacy.example.com", Mode: "sni_only", Enabled: true},
		{Name: "disabled.example.com", Mode: "sni_only", Enabled: false},
		{Name: "relay.example.com", Mode: "relay", Enabled: true},
		{Name: "certificate.example.com", Mode: "cert_only", Enabled: true},
	}}
	got := coverSiteSummariesFromConfig(7, cfg)
	if len(got) != 2 {
		t.Fatalf("got %d editable SNI sites, want 2: %#v", len(got), got)
	}
	if got[0].NodeID != 7 || got[0].Domain != "notes.example.com" ||
		got[0].SiteCatalog != "builtin" || got[0].SiteTemplate != "notes" {
		t.Errorf("incorrect explicit website: %#v", got[0])
	}
	if got[1].NodeID != 7 || got[1].Domain != "legacy.example.com" ||
		got[1].SiteCatalog != "builtin" || got[1].SiteTemplate != "notes" {
		t.Errorf("legacy website did not receive safe defaults: %#v", got[1])
	}
}
