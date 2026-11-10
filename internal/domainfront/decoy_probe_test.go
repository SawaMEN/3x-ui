package domainfront

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicDecoyIPRejectsLocalAndReservedNetworks(t *testing.T) {
	for _, tc := range []struct {
		address string
		allowed bool
	}{
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"9.9.9.9", true},
		{"10.1.2.3", false},
		{"127.0.0.1", false},
		{"172.16.0.1", false},
		{"192.168.0.1", false},
		{"169.254.169.254", false},
		{"100.64.5.7", false},
		{"198.18.0.1", false},
		{"192.0.2.20", false},
		{"203.0.113.123", false},
		{"::1", false},
		{"fc00::1", false},
		{"fe80::1", false},
		{"2001:db8::1", false},
		{"64:ff9b::a00:1", false},
		{"64:ff9b::7f00:1", false},
		{"64:ff9b:1::808:808", false},
		{"2002:7f00:1::1", false},
		{"fec0::1", false},
		{"64:ff9b::808:808", true},
	} {
		if got := publicDecoyIP(net.ParseIP(tc.address)); got != tc.allowed {
			t.Errorf("%s public = %v, want %v", tc.address, got, tc.allowed)
		}
	}
}

func TestRankDecoySitesFastestWorkingHTTPSFirst(t *testing.T) {
	sites := []DecoySiteCheck{
		{URL: "https://down.example/", Available: false, LatencyMS: 0},
		{URL: "https://slow.example/", Available: true, LatencyMS: 180, SNILatencyMS: 20},
		{URL: "https://fast.example/", Available: true, LatencyMS: 40, SNILatencyMS: 30},
		{URL: "https://tie.example/", Available: true, LatencyMS: 40, SNILatencyMS: 80},
	}
	rankDecoySites(sites)
	for i, want := range []string{
		"https://fast.example/", "https://tie.example/",
		"https://slow.example/", "https://down.example/",
	} {
		if sites[i].URL != want {
			t.Fatalf("ranking at %d: got %+v, want %q", i, sites, want)
		}
	}
}

func TestProbeDecoySiteChecksRootAndNestedPath(t *testing.T) {
	requested := []string{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requested = append(requested, r.URL.Path)
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	probe := probeDecoySite(context.Background(), srv.Client(), DecoyCandidate{URL: srv.URL + "/", SNILatencyMS: 33})
	if !probe.Available || probe.Status != 200 || probe.PathStatus != 404 || probe.SNILatencyMS != 33 {
		t.Fatalf("unexpected scan: %+v", probe)
	}
	if strings.Join(requested, ",") != "/,/123" {
		t.Fatalf("unexpected probe paths: %v", requested)
	}
}

func TestDecoyClientDoesNotFollowRedirects(t *testing.T) {
	client := decoyHTTPClient()
	if err := client.CheckRedirect(nil, nil); !errors.Is(err, http.ErrUseLastResponse) {
		t.Fatalf("redirect policy = %v", err)
	}
}

func TestSiteCandidatesRejectUnsafeURLs(t *testing.T) {
	result := ScanDecoySites(context.Background(), []DecoyCandidate{
		{URL: "http://example.com/"},
		{URL: "https://example.com:8443/"},
		{URL: "https://127.0.0.1/"},
		{URL: "https://example.com/private"},
		{URL: "https://alice:secret@example.com/"},
		{URL: "file:///etc/passwd"},
	})
	if len(result.Sites) != 0 || result.Recommended != "" {
		t.Fatalf("accepted unsafe sites: %+v", result)
	}
}
