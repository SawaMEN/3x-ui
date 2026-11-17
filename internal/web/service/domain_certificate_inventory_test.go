package service

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

func TestCertificateChallengeClassification(t *testing.T) {
	tests := []struct {
		name, webroot, expected string
	}{
		{"http", "/var/www/example", "http-01"},
		{"standalone", "no", "http-01"},
		{"alpn", "alpn", "tls-alpn-01"},
		{"cloudflare", "dns_cf", "dns-01"},
		{"unknown", "", "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if actual := acmeSHChallenge(tc.webroot); actual != tc.expected {
				t.Fatalf("challenge = %q, want %q", actual, tc.expected)
			}
		})
	}
	if actual := certbotAuthenticator([]byte("[renewalparams]\nauthenticator = dns-cloudflare\n")); actual != "dns-cloudflare" {
		t.Fatalf("Certbot authenticator = %q", actual)
	}
}

func TestCertificateFirewallPorts(t *testing.T) {
	for _, tc := range []struct {
		challenge string
		want      []int
	}{
		{"dns-01", []int{}},
		{"none", []int{}},
		{"unknown", []int{}},
		{"http-01", []int{80}},
		{"tls-alpn-01", []int{443}},
		{"http-01+tls-alpn-01", []int{80, 443}},
	} {
		if got := challengeFirewallPorts(tc.challenge, 80, 443); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s ports = %v, want %v", tc.challenge, got, tc.want)
		}
	}
}

func TestCertificateInventoryGroupsSharedFilesAndSkipsRemotePorts(t *testing.T) {
	status := domainfront.DomainFrontStatus{Config: domainfront.DefaultConfig(), Certificates: []domainfront.DomainFrontCertificate{
		{Name: "a.example.org", CertFile: "/tmp/missing-shared-cert.pem", KeyFile: "/tmp/missing-shared-key.pem", Source: "panel", Renewal: "external", State: "valid"},
		{Name: "b.example.org", CertFile: "/tmp/missing-shared-cert.pem", KeyFile: "/tmp/missing-shared-key.pem", Source: "subscription", Renewal: "external", State: "valid"},
		{Name: "remote.example.org", Source: "remote", State: "external"},
	}}
	inventory := makeCertificateInventory(status, nil, nil)
	if len(inventory) != 2 {
		t.Fatalf("inventory contains %d items, expected 2", len(inventory))
	}
	for _, item := range inventory {
		if item.Manager == "remote" && len(item.Ports) > 0 {
			t.Fatalf("remote SNI opened local firewall: %+v", item)
		}
		if item.Manager == "external" && !reflect.DeepEqual(item.UsedBy, []string{"panel", "subscription"}) {
			t.Fatalf("shared certificate was not grouped: %+v", item)
		}
	}
}

func TestRemoteCertificateReferencesDoNotExposeNodeKeys(t *testing.T) {
	sites := []ManagedSNIDomain{
		{Name: "node.example.org", Source: "remote", NodeID: 9, NodeName: "Edge", CertFile: "/node/private/cert.pem", KeyFile: "/node/private/key.pem"},
		{Name: "local.example.org", Source: "gateway", CertFile: "/local/cert.pem"},
	}
	items := appendRemoteCertificateReferences(nil, sites)
	if len(items) != 1 || items[0].NodeID != 9 || items[0].CertFile != "" || len(items[0].Ports) != 0 {
		t.Fatalf("remote node leaked paths or ports: %+v", items)
	}
}

func TestACMEShWebrootOpensActualLocalListener(t *testing.T) {
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.HTTPPort = 8080
	cfg.Domains = []domainfront.DomainFrontDomain{
		{Name: "sub.example.org", Enabled: true, Mode: "sub_link_only", TLS: "auto"},
	}
	cert := domainfront.DomainFrontCertificate{
		Name: "sub.example.org", Source: "panel", CertFile: "/tmp/sub.pem",
		KeyFile: "/tmp/sub.key", Renewal: "acme.sh", State: "valid",
	}
	status := domainfront.DomainFrontStatus{Config: cfg, Running: true, Certificates: []domainfront.DomainFrontCertificate{cert}}
	states := []domainACME{{values: map[string]string{
		"Le_Domain":            "sub.example.org",
		"Le_RealFullChainPath": "/tmp/sub.pem",
		"Le_Webroot":           filepath.Join(config.GetDBFolderPath(), "domain-acme-webroot"),
	}}}
	items := makeCertificateInventory(status, states, nil)
	if len(items) != 1 || !reflect.DeepEqual(items[0].Ports, []int{8080}) {
		t.Fatalf("webroot renewal should open the local listener, got %+v", items)
	}
	status.Running = false
	items = makeCertificateInventory(status, states, nil)
	if len(items) != 1 || !reflect.DeepEqual(items[0].Ports, []int{80}) {
		t.Fatalf("inactive webroot must not claim the gateway listener: %+v", items)
	}
}
