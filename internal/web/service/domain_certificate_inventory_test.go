package service

import (
	"reflect"
	"testing"

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
