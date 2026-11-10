package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

func TestNodeCertificateInventoryOnlyPublishesMetadata(t *testing.T) {
	remote := runtime.RemoteCertificateSummary{
		ID: "old-node-local-id", Name: " Sub.Example.Org. ",
		Names:   []string{"sub.example.org", "www.example.org"},
		Manager: "certbot", Challenge: "http-01",
		State: "valid", Issuer: "Issuer", ExpiresAt: 1890000000,
		DaysLeft: 25, AutoRenew: true,
		Warning: "token=should-never-be-copied",
	}
	got, ok := nodeCertificateInventoryItem(7, "Node-7", remote)
	if !ok || got.Name != "sub.example.org" || got.Source != "remote" || got.NodeID != 7 ||
		got.NodeName != "Node-7" || got.Manager != "certbot" || got.Issuer != "Issuer" {
		t.Fatalf("unexpected remote certificate: %+v, ok=%v", got, ok)
	}
	if got.CertFile != "" || got.Warning != "" || len(got.Ports) != 0 {
		t.Fatalf("remote secrets, paths or firewall policy leaked: %+v", got)
	}
	again, ok := nodeCertificateInventoryItem(8, "Node-8", remote)
	if !ok || again.ID == got.ID {
		t.Fatal("node-qualified certificate IDs must be distinct")
	}
}

func TestNodeCertificateInventorySanitizesUntrustedPeerMetadata(t *testing.T) {
	tests := []string{
		"localhost", "../example.org", "internal.org/path", "evil..example.org",
		"-example.org", "example.org\nsecret", "",
	}
	for _, name := range tests {
		if _, ok := nodeCertificateInventoryItem(1, "Node", runtime.RemoteCertificateSummary{Name: name}); ok {
			t.Fatalf("accepted invalid name: %q", name)
		}
	}
	item, ok := nodeCertificateInventoryItem(2, "Node", runtime.RemoteCertificateSummary{
		ID: "x", Name: "good.example.org", Manager: "unexpected",
		Challenge: "malicious", State: "unexpected",
		Warning: "token", Issuer: "Trusted",
	})
	if !ok || item.Manager != "external" || item.Challenge != "unknown" || item.State != "external" {
		t.Fatalf("peer metadata not safely normalized: %+v", item)
	}
}

func TestRemoteCertificateRefDoesNotDuplicateNodeInventory(t *testing.T) {
	cert := CertificateInventoryItem{ID: "remote-cert", Name: "sub.example.org", NodeID: 17, NodeName: "edge", Source: "remote"}
	sites := []ManagedSNIDomain{
		{NodeID: 17, NodeName: "edge", Name: "sub.example.org", Source: "remote"},
		{NodeID: 18, NodeName: "other", Name: "sub.example.org", Source: "remote"},
	}
	combined := appendRemoteCertificateReferences([]CertificateInventoryItem{cert}, sites)
	if len(combined) != 2 {
		t.Fatalf("expected one real remote item and one fallback reference, got %+v", combined)
	}
}
