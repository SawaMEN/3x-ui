package service

import "testing"

func TestManagedSNIDomainsSameHostnameAcrossNodes(t *testing.T) {
	seen := map[string]bool{"vpn.example.org": true}
	panel := []ManagedSNIDomain{{
		Name:     "vpn.example.org",
		Source:   "gateway",
		CertFile: "/panel/cert.pem",
		KeyFile:  "/panel/key.pem",
	}}
	remote := []ManagedSNIDomain{
		{NodeID: 7, NodeName: "One", Name: "vpn.example.org", Source: "remote", CertFile: "/node-7/cert.pem", KeyFile: "/node-7/key.pem"},
		{NodeID: 9, NodeName: "Two", Name: "VPN.EXAMPLE.ORG.", Source: "remote", CertFile: "/node-9/cert.pem", KeyFile: "/node-9/key.pem", ExpiresAt: 2000000000, Issuer: "Test CA"},
		{NodeID: 7, NodeName: "One", Name: "vpn.example.org", Source: "remote", CertFile: "/duplicate.pem", KeyFile: "/duplicate.key"},
		{NodeID: 10, NodeName: "Three", Name: "vpn.example.org", Source: "remote"},
		{NodeID: 0, Name: "vpn.example.org", Source: "remote", CertFile: "/unowned.pem", KeyFile: "/unowned.key"},
	}
	got := mergeNodeManagedSNIDomains(panel, remote, seen)
	if len(got) != 3 {
		t.Fatalf("expected panel and 2 distinct node certificates, got %+v", got)
	}
	if got[0].CertFile != "/panel/cert.pem" ||
		got[1].NodeID != 7 || got[1].CertFile != "/node-7/cert.pem" ||
		got[2].NodeID != 9 || got[2].CertFile != "/node-9/cert.pem" ||
		got[2].ExpiresAt != 2000000000 || got[2].Issuer != "Test CA" {
		t.Fatalf("certificate paths were mixed, lost or duplicated: %+v", got)
	}
	if !seen["vpn.example.org"] {
		t.Fatal("manual domain fallback must not supersede an active certificate")
	}
}

func TestManagedSNIDomainsRejectsInvalidRemoteCertificateReferences(t *testing.T) {
	seen := map[string]bool{}
	got := mergeNodeManagedSNIDomains(nil, []ManagedSNIDomain{
		{NodeID: 3, Name: "127.0.0.1", Source: "remote", CertFile: "/a", KeyFile: "/b"},
		{NodeID: 3, Name: "bad", Source: "remote", CertFile: "/a", KeyFile: "/b"},
		{NodeID: 3, Name: "valid.example.org", Source: "local", CertFile: "/a", KeyFile: "/b"},
		{NodeID: 3, Name: "valid.example.org", Source: "remote", CertFile: "/a", KeyFile: ""},
	}, seen)
	if len(got) != 0 || len(seen) != 0 {
		t.Fatalf("invalid remote certificate references were accepted: %+v", got)
	}
}

func TestManagedSNIDomainsKeepsSeveralDomainsOnOneNode(t *testing.T) {
	seen := map[string]bool{}
	sites := []ManagedSNIDomain{
		{NodeID: 4, Name: "first.example.org", Source: "remote", CertFile: "/first.crt", KeyFile: "/first.key"},
		{NodeID: 4, Name: "second.example.org", Source: "remote", CertFile: "/second.crt", KeyFile: "/second.key"},
		{NodeID: 5, Name: "first.example.org", Source: "remote", CertFile: "/node-5.crt", KeyFile: "/node-5.key"},
	}
	got := mergeNodeManagedSNIDomains(nil, sites, seen)
	if len(got) != 3 || len(seen) != 2 {
		t.Fatalf("distinct (node, hostname) sites collapsed: %+v", got)
	}
}

func TestManagedNodeCertificateSourceSupported(t *testing.T) {
	for _, source := range []string{"telemt", "gateway", "panel", "subscription"} {
		if !managedNodeCertificateSourceSupported(source) {
			t.Errorf("active node certificate source %q was rejected", source)
		}
	}
	for _, source := range []string{"remote", "", "unknown"} {
		if managedNodeCertificateSourceSupported(source) {
			t.Errorf("unsupported node certificate source %q was trusted", source)
		}
	}
}
