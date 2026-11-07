package service

import (
	"context"
	"strings"
	"testing"
)

func TestNearbyRedirectCandidatesOnlyPublicVerifiedWebsites(t *testing.T) {
	rows := []*RealityScanResult{
		{Host: "slow.example.com", IP: "8.8.8.7", Port: 443, CertValid: true, LatencyMs: 140},
		{Host: "FAST.example.com", IP: "8.8.8.9", Port: 443, CertValid: true, LatencyMs: 30},
		{Host: "fast.example.com", IP: "8.8.8.11", Port: 443, CertValid: true, LatencyMs: 31},
		{Host: "private.example.com", IP: "10.0.0.1", Port: 443, CertValid: true, LatencyMs: 1},
		{Host: "unverified.example.com", IP: "8.8.8.12", Port: 443},
		{Host: "wrongport.example.com", IP: "8.8.8.13", Port: 8443, CertValid: true},
		{Host: "8.8.8.14", IP: "8.8.8.14", Port: 443, CertValid: true},
		{Host: "wildcard.*.example.com", IP: "8.8.8.15", Port: 443, CertValid: true},
		{Host: "blocked.example.com", IP: "8.8.8.16", Port: 443, CertValid: true, PrivateTarget: true},
		nil,
	}
	got := nearbyRedirectCandidates(rows)
	if len(got) != 2 {
		t.Fatalf("expected two real neighboring websites: %+v", got)
	}
	if got[0].URL != "https://fast.example.com/" || got[0].SNILatencyMS != 30 ||
		got[1].URL != "https://slow.example.com/" {
		t.Fatalf("should rank usable names by measured SNI time: %+v", got)
	}
}

func TestNearbyRedirectCandidatesBoundedAndFastestFirst(t *testing.T) {
	var rows []*RealityScanResult
	for i := 0; i < 30; i++ {
		host := strings.Repeat("a", i%20+1) + "b.example.com"
		rows = append(rows, &RealityScanResult{
			Host: host, IP: "8.8.8.9", Port: 443, CertValid: true,
			LatencyMs: 30 - i,
		})
	}
	got := nearbyRedirectCandidates(rows)
	if len(got) != redirectHTTPFinalists {
		t.Fatalf("HTTP shortlist not bounded: %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].SNILatencyMS > got[i].SNILatencyMS {
			t.Fatalf("incorrect latency ordering: %+v", got)
		}
	}
}

func TestNearbyRedirectScanRejectsInvalidRange(t *testing.T) {
	for _, limit := range []int{-1, 0, realityDiscoverMaxIPs + 1} {
		if _, err := (&ServerService{}).ScanNearbySNIsLimited(context.Background(), "8.8.8.8", limit); err == nil {
			t.Fatalf("accepted invalid scan limit %d", limit)
		}
	}
}
