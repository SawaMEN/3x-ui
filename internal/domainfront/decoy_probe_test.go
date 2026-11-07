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
	} {
		if got := publicDecoyIP(net.ParseIP(tc.address)); got != tc.allowed {
			t.Errorf("%s public = %v, want %v", tc.address, got, tc.allowed)
		}
	}
}

func TestScoreDecoySite(t *testing.T) {
	for _, tc := range []struct {
		status, pathStatus int
		latency            int64
		available          bool
		score              int
	}{
		{200, 200, 50, true, 89},
		{302, 404, 100, true, 73},
		{200, 403, 50, true, 64},
		{403, 200, 10, false, 0},
		{0, 0, 10, false, 0},
	} {
		site := &DecoySiteCheck{Status: tc.status, PathStatus: tc.pathStatus, LatencyMS: tc.latency}
		scoreDecoySite(site)
		if site.Available != tc.available || site.Score != tc.score {
			t.Errorf("%+v: got available %v, score %d", tc, site.Available, site.Score)
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

	probe := probeDecoySite(context.Background(), srv.Client(), DecoyCandidate{URL: srv.URL+"/", TLS13: true, HTTP2: true, SNIStable: true})
	if !probe.Available || probe.Status != 200 || probe.PathStatus != 404 || probe.Score < 65 {
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
