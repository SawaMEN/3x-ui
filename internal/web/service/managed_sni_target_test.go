package service

import "testing"

func TestManagedSNIRealityTarget(t *testing.T) {
	for _, test := range []struct {
		name string
		port int
		want string
	}{
		{"vpn.example.org", 8443, "vpn.example.org:8443"},
		{"vpn.example.org", 0, ""}, // Certificate-only: no HTTPS endpoint.
		{"vpn.example.org", 65536, ""},
		{"*.example.org", 443, ""},
		{"127.0.0.1", 443, ""},
	} {
		if got := managedSNIRealityTarget(test.name, test.port); got != test.want {
			t.Errorf("target(%q, %d) = %q, want %q", test.name, test.port, got, test.want)
		}
	}
}

func TestRemoteManagedSNIRealityTarget(t *testing.T) {
	sites := []ManagedSNIDomain{
		{Name: "vpn.example.org", Source: "remote", NodeID: 1, RealityTarget: "VPN.EXAMPLE.ORG.:9443"},
		{Name: "cert.example.org", Source: "remote", NodeID: 1},
		{Name: "bad.example.org", Source: "remote", NodeID: 1, RealityTarget: "other.example.org:443"},
	}
	got := mergeNodeManagedSNIDomains(nil, sites, map[string]bool{})
	if len(got) != 3 || got[0].RealityTarget != "vpn.example.org:9443" || got[1].RealityTarget != "" || got[2].RealityTarget != "" {
		t.Fatalf("endpoint metadata lost or certificate treated as endpoint: %+v", got)
	}
}
