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
		{NodeID: 9, NodeName: "Two", Name: "VPN.EXAMPLE.ORG.", Source: "remote", CertFile: "/node-9/cert.pem", KeyFile: "/node-9/key.pem"},
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
		got[2].NodeID != 9 || got[2].CertFile != "/node-9/cert.pem" {
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
