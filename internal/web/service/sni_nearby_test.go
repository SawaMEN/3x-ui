package service

import (
	"context"
	"encoding/pem"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNearbySNIAddresses(t *testing.T) {
	for _, tc := range []struct {
		origin string
		limit  int
		want   []string
	}{
		{"8.8.8.8", 4, []string{"8.8.8.7", "8.8.8.9", "8.8.8.6", "8.8.8.10"}},
		{"::ffff:8.8.8.8", 3, []string{"8.8.8.7", "8.8.8.9", "8.8.8.6"}},
		{"2606:4700:4700::1111", 2, []string{"2606:4700:4700::1110", "2606:4700:4700::1112"}},
		{"127.0.0.1", 4, nil}, {"10.0.0.1", 4, nil}, {"::1", 4, nil},
		{"8.8.8.8", 0, nil}, {"224.0.0.1", 4, nil}, {"ff02::1", 4, nil},
	} {
		if got := nearbySNIAddresses(netip.MustParseAddr(tc.origin), tc.limit); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: got %v want %v", tc.origin, got, tc.want)
		}
	}
	if got := nearbySNIAddresses(netip.Addr{}, 5); len(got) != 0 {
		t.Fatal(got)
	}
	if got := nearbySNIAddresses(netip.MustParseAddr("8.8.8.8"), 256); len(got) != 256 {
		t.Fatal(len(got))
	}
}

func TestNearbySNIVerifiesDiscoveredNameAndPreservesIP(t *testing.T) {
	probe := func(ip string, port int, sni string, timeout time.Duration, xver int, private bool) *RealityScanResult {
		if private || xver != 0 || port != 8443 {
			t.Error("unexpected probe options")
		}
		if sni == "" {
			return &RealityScanResult{Host: "edge.example", CertChainValid: true}
		}
		if sni != "edge.example" {
			t.Errorf("unexpected SNI %q", sni)
		}
		return &RealityScanResult{Target: "edge.example:8443", Host: sni, CertValid: ip != "8.8.8.9", Feasible: true, LatencyMs: 5}
	}
	got, err := scanNearbySNIAddresses(context.Background(), []string{"8.8.8.7", "8.8.8.9"}, 8443, probe)
	if err != nil || len(got) != 1 {
		t.Fatalf("%v %v", got, err)
	}
	if !reflect.DeepEqual(got[0].ServerNames, []string{"edge.example"}) {
		t.Fatalf("unverified server names: %v", got[0].ServerNames)
	}
	if got[0].Target != "8.8.8.7:8443" || got[0].IP != "8.8.8.7" || got[0].Host != "edge.example" {
		t.Fatalf("%+v", got[0])
	}
}

func TestNearbySNISkipsUnusableDiscoveryAndHonorsCancellation(t *testing.T) {
	for _, discovery := range []*RealityScanResult{
		nil, {Host: "untrusted.example"}, {Host: "*.example", CertChainValid: true}, {Host: "8.8.8.7", CertChainValid: true},
	} {
		calls := 0
		got, err := scanNearbySNIAddresses(context.Background(), []string{"8.8.8.7"}, 443, func(string, int, string, time.Duration, int, bool) *RealityScanResult { calls++; return discovery })
		if err != nil || len(got) != 0 || calls != 1 {
			t.Fatalf("%v %v calls=%d", got, err, calls)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := scanNearbySNIAddresses(ctx, []string{"8.8.8.7"}, 443, func(string, int, string, time.Duration, int, bool) *RealityScanResult {
		t.Error("probe after cancellation")
		return nil
	})
	if err != context.Canceled {
		t.Fatal(err)
	}
}

func TestNearbySNIRejectsPrivateReference(t *testing.T) {
	for _, ref := range []string{"127.0.0.1", "10.0.0.1", "[::1]:443", "8.8.8.0/24", "a,b"} {
		if _, err := (&ServerService{}).ScanNearbySNIs(context.Background(), ref); err == nil {
			t.Errorf("accepted %s", ref)
		}
	}
}

func TestNearbySNIFallsBackToSANAndPrefersFeasible(t *testing.T) {
	var names []string
	got, err := scanNearbySNIAddresses(context.Background(), []string{"8.8.8.7"}, 443,
		func(ip string, port int, sni string, timeout time.Duration, xver int, private bool) *RealityScanResult {
			if sni == "" {
				return &RealityScanResult{Host: "old.example", CertChainValid: true, ServerNames: []string{"old.example", "tls.example", "reality.example"}}
			}
			names = append(names, sni)
			return &RealityScanResult{Host: sni, CertValid: sni != "old.example", Feasible: sni == "reality.example"}
		})
	if err != nil || len(got) != 1 || got[0].Host != "reality.example" {
		t.Fatalf("%+v %v", got, err)
	}
	if !reflect.DeepEqual(names, []string{"old.example", "tls.example", "reality.example"}) {
		t.Fatal(names)
	}
}

func TestNearbySNIRetryBoundAndConcurrency(t *testing.T) {
	var active, peak, calls atomic.Int32
	addresses := make([]string, realityScanConcurrency*2)
	for i := range addresses {
		addresses[i] = "8.8.8.7"
	}
	_, err := scanNearbySNIAddresses(context.Background(), addresses, 443,
		func(ip string, port int, sni string, timeout time.Duration, xver int, private bool) *RealityScanResult {
			n := active.Add(1)
			defer active.Add(-1)
			for {
				prev := peak.Load()
				if n <= prev || peak.CompareAndSwap(prev, n) {
					break
				}
			}
			time.Sleep(time.Millisecond)
			if sni == "" {
				return &RealityScanResult{Host: "first.example", CertChainValid: true, ServerNames: []string{"a.example", "b.example", "c.example", "d.example", "e.example", "f.example", "g.example", "h.example", "i.example"}}
			}
			calls.Add(1)
			return &RealityScanResult{Host: sni}
		})
	if err != nil || peak.Load() > realityScanConcurrency || calls.Load() != int32(len(addresses)*8) {
		t.Fatalf("err=%v peak=%d calls=%d", err, peak.Load(), calls.Load())
	}
}

func TestRealityProbeCancellationStopsActiveHandshake(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	ready := make(chan struct{})
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		b := make([]byte, 4096)
		if _, err := conn.Read(b); err == nil {
			close(ready)
		}
		_, _ = conn.Read(b)
	}()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan *RealityScanResult, 1)
	port := listener.Addr().(*net.TCPAddr).Port
	go func() {
		result <- (&ServerService{}).probeRealityAddrContext(ctx, "127.0.0.1", port, "example.com", 10*time.Second, 0, true)
	}()
	select {
	case <-ready:
	case <-time.After(2 * time.Second):
		t.Fatal("probe did not start TLS")
	}
	cancel()
	select {
	case r := <-result:
		if r.Feasible || !strings.Contains(r.Reason, "canceled") {
			t.Fatalf("%+v", r)
		}
	case <-time.After(time.Second):
		t.Fatal("cancellation left TLS probe running")
	}
}

func TestNearbyTLSProbeEndToEnd(t *testing.T) {
	if address := os.Getenv("SNI_TEST_TLS_ADDRESS"); address != "" {
		host, portString, err := net.SplitHostPort(address)
		if err != nil {
			t.Fatal(err)
		}
		port, _ := strconv.Atoi(portString)
		var sentNames []string
		got, err := scanNearbySNIAddresses(context.Background(), []string{host}, port,
			func(ip string, port int, sni string, timeout time.Duration, xver int, private bool) *RealityScanResult {
				sentNames = append(sentNames, sni)
				// Loopback is permitted only for this local TLS fixture.
				probeCtx := context.Background()
				if sni != "" {
					probeCtx = context.WithValue(probeCtx, sniHTTPCheckKey{}, true)
				}
				return (&ServerService{}).probeRealityAddrContext(probeCtx, ip, port, sni, timeout, xver, true)
			})
		if err != nil || len(got) != 1 {
			t.Fatalf("real TLS discovery failed: %+v %v", got, err)
		}
		r := got[0]
		if r.HTTP2OK == nil || !*r.HTTP2OK || r.TLSMs <= 0 {
			t.Fatalf("missing detailed TLS/HTTP2 diagnostics: %+v", r)
		}
		if !r.Feasible || !r.CertValid || r.Target != address || r.IP != host || !reflect.DeepEqual(r.ServerNames, []string{r.Host}) {
			t.Fatalf("unexpected result: %+v", r)
		}
		if !reflect.DeepEqual(sentNames, []string{"", r.Host}) {
			t.Fatalf("TLS SNI sequence: %v", sentNames)
		}
		return
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	server.EnableHTTP2 = true
	server.StartTLS()
	defer server.Close()
	dir := t.TempDir()
	certPath := filepath.Join(dir, "root.pem")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, binary, "-test.run=^TestNearbyTLSProbeEndToEnd$", "-test.count=1")
	child.Env = append(os.Environ(), "SNI_TEST_TLS_ADDRESS="+server.Listener.Addr().String(), "SSL_CERT_FILE="+certPath, "SSL_CERT_DIR="+dir)
	if output, err := child.CombinedOutput(); err != nil {
		t.Fatalf("TLS fixture: %v\n%s", err, output)
	}
}
