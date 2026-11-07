package service

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestRedirectSNITargetsReuseVerifiedHistoryAndConfiguredList(t *testing.T) {
	dnsMatch := true
	certified := &RealityScanResult{
		Host:         "verified.example.com",
		IP:           "8.8.8.8",
		Port:         443,
		CertValid:    true,
		Attempts:     5,
		Successes:    5,
		DNSMatchesIP: &dnsMatch,
		CheckedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	unstable := *certified
	unstable.Host, unstable.Successes = "unstable.example.com", 3
	private := *certified
	private.Host, private.PrivateTarget = "private.example.com", true
	otherPort := *certified
	otherPort.Host, otherPort.Port = "alt.example.com", 8443
	dnsMismatch := false
	mismatch := *certified
	mismatch.Host = "mismatch.example.com"
	mismatch.DNSMatchesIP = &dnsMismatch

	list := redirectSNITargets([]string{
		"another.example.com:443",
		"10.0.0.1:443",
		"internal.example.com:8443",
		"verified.example.com:443",
		"192.0.2.0/30",
		"example.org:443",
	}, []SNIJobSnapshot{{
		State:   "done",
		Results: []*RealityScanResult{certified, &unstable, &private, &otherPort, &mismatch},
	}})
	got := strings.Join(list, ",")
	if len(list) != 3 || list[0] != "verified.example.com:443" ||
		!strings.Contains(got, "another.example.com:443") ||
		!strings.Contains(got, "example.org:443") {
		t.Fatalf("unexpected SNI shortlist: %v", list)
	}
	for _, value := range list {
		if strings.Contains(value, "internal") || strings.Contains(value, "private") ||
			strings.Contains(value, "unstable") || strings.Contains(value, "mismatch") ||
			net.ParseIP(strings.TrimSuffix(value, ":443")) != nil {
			t.Fatalf("invalid historical or configured candidate %q", value)
		}
	}
	if !trustedRedirectHistory([]SNIJobSnapshot{{Results: []*RealityScanResult{certified}}})["verified.example.com"] {
		t.Fatal("recent stable SNI result not recognized")
	}
}

func TestRedirectSNITargetsBoundedSampleOfSharedCatalog(t *testing.T) {
	var configured []string
	for i := 0; i < 200; i++ {
		configured = append(configured, "host"+strings.Repeat("x", i%12)+string(rune('a'+i%26))+".example.com:443")
	}
	got := redirectSNITargets(configured, nil)
	if len(got) > redirectMaxSNICandidates || len(got) < 10 {
		t.Fatalf("unexpected shortlist size %d: %v", len(got), got)
	}
	seen := map[string]bool{}
	for _, row := range got {
		if seen[row] {
			t.Fatalf("duplicate %q", row)
		}
		seen[row] = true
	}
}
