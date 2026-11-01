package service

import (
	"context"
	"errors"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func waitSNIJob(t *testing.T, m *sniJobManager, id string) SNIJobSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		view, err := m.get(id, false)
		if err != nil {
			t.Fatal(err)
		}
		if view.State == "done" || view.State == "failed" || view.State == "canceled" {
			return view
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("job did not finish")
	return SNIJobSnapshot{}
}
func TestSNIJobsQueueAndCancellation(t *testing.T) {
	m := newSNIJobManager(func(ctx context.Context, j *sniJob) error { <-ctx.Done(); return ctx.Err() }, nil)
	var ids []string
	for i := 0; i < 4; i++ {
		view, err := m.start(SNIScanRequest{Mode: "candidates"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, view.ID)
	}
	if _, err := m.start(SNIScanRequest{Mode: "candidates"}); err == nil {
		t.Fatal("unbounded queue")
	}
	for _, id := range ids {
		m.get(id, true)
	}
	for _, id := range ids {
		if view := waitSNIJob(t, m, id); view.State != "canceled" {
			t.Fatal(view.State)
		}
	}
	m.close()
}
func TestSNIJobsPublishAndPersistBeforeDone(t *testing.T) {
	saved := make(chan SNIJobSnapshot, 1)
	m := newSNIJobManager(func(ctx context.Context, j *sniJob) error {
		j.publish(3, 1, &RealityScanResult{Target: "8.8.8.8:443", Host: "edge.example", CertValid: true, Feasible: true})
		return nil
	}, func(view SNIJobSnapshot) error { saved <- view; return nil })
	view, err := m.start(SNIScanRequest{Mode: "candidates"})
	if err != nil {
		t.Fatal(err)
	}
	result := waitSNIJob(t, m, view.ID)
	if result.State != "done" || result.Total != 3 || result.Completed != 1 || len(result.Results) != 1 {
		t.Fatalf("%+v", result)
	}
	select {
	case stored := <-saved:
		if stored.ID != view.ID {
			t.Fatal(stored.ID)
		}
	default:
		t.Fatal("done published before persistence")
	}
	m.close()
}
func TestSNIJobsReportSaveFailureAndRejectInvalidRequests(t *testing.T) {
	m := newSNIJobManager(func(context.Context, *sniJob) error { return nil }, func(SNIJobSnapshot) error { return errors.New("disk full") })
	for _, req := range []SNIScanRequest{{Mode: "bad"}, {Mode: "recheck"}, {Purpose: "bad"}, {Mode: "recheck", Recheck: []*SNISavedTarget{nil}}, {Mode: "recheck", Recheck: []*SNISavedTarget{{IP: "domain.example", Host: "example", Port: 443}}}} {
		if _, err := m.start(req); err == nil {
			t.Fatalf("accepted %+v", req)
		}
	}
	view, err := m.start(SNIScanRequest{Mode: "candidates"})
	if err != nil {
		t.Fatal(err)
	}
	result := waitSNIJob(t, m, view.ID)
	if result.Error == "" {
		t.Fatal("save failure hidden")
	}
	m.close()
}
func TestSNIStabilityMeasurementsAndDNS(t *testing.T) {
	row := &RealityScanResult{IP: "8.8.8.8", Target: "8.8.8.8:443", Host: "edge.example", Port: 443, CertValid: true, Feasible: true}
	index := 0
	samples := []float64{10, 100, 12, 11, 0}
	out := measureSNICandidate(context.Background(), row, "reality", func(ctx context.Context, ip string, port int, sni string) *RealityScanResult {
		i := index
		index++
		if ip != row.IP || sni != row.Host {
			t.Error("lost pinned target")
		}
		if i == 4 {
			if check, _ := ctx.Value(sniHTTPCheckKey{}).(bool); !check {
				t.Error("no HTTP2 diagnostic requested")
			}
			return &RealityScanResult{}
		}
		return &RealityScanResult{CertValid: true, Feasible: true, TCPMs: samples[i], TLSMs: 1}
	}, func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	})
	if out.Attempts != 5 || out.Successes != 4 || out.MedianMs != 12.5 || out.JitterMs != 90 || out.DNSMatchesIP == nil || !*out.DNSMatchesIP || out.CheckedAt == "" {
		t.Fatalf("%+v", out)
	}
	if row.Attempts != 0 {
		t.Fatal("mutated published result")
	}
	unstable := *out
	unstable.Successes = 3
	unstable.MedianMs = 1
	rows := []*RealityScanResult{&unstable, out}
	rankSNIResults(rows, "reality")
	if rows[0] != out {
		t.Fatal("unstable target ranked first")
	}
}
func TestSNIHistoryPersistsAndIsBounded(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "sni-history.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.CloseDB() })
	for i := 0; i < 25; i++ {
		if err := saveSNIHistory(SNIJobSnapshot{ID: time.Unix(int64(i), 0).String(), Results: func() []*RealityScanResult {
			rows := make([]*RealityScanResult, 25)
			for j := range rows {
				rows[j] = &RealityScanResult{IP: "8.8.8.8", Host: "edge.example", CertValid: true}
			}
			return rows
		}()}); err != nil {
			t.Fatal(err)
		}
	}
	if err := database.CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := database.InitDB(dbPath); err != nil {
		t.Fatal(err)
	}
	rows, err := SNIScanHistory()
	if err != nil || len(rows) != 20 || len(rows[0].Results) != 20 {
		t.Fatalf("history=%d err=%v", len(rows), err)
	}
}

func TestSNIJobPinsAndDeduplicatesByIPAndSNI(t *testing.T) {
	row := &RealityScanResult{Target: "edge.example:443", Host: "edge.example", IP: "8.8.8.8", Port: 443, CertValid: true, ServerNames: []string{"unverified.example"}}
	pinned := pinSNIJobResult(row)
	if pinned.Target != "8.8.8.8:443" || len(pinned.ServerNames) != 1 || pinned.ServerNames[0] != "edge.example" || row.Target != "edge.example:443" {
		t.Fatalf("%+v", pinned)
	}
	other := *pinned
	other.Host = "other.example"
	rows := dedupSNIJobResults([]*RealityScanResult{pinned, pinned, &other})
	if len(rows) != 2 {
		t.Fatal("lost an independent SNI on the same IP")
	}
}

func TestSNIQueuedCancellationFreesCapacityImmediately(t *testing.T) {
	started := make(chan struct{})
	var once sync.Once
	m := newSNIJobManager(func(ctx context.Context, j *sniJob) error {
		once.Do(func() { close(started) })
		<-ctx.Done()
		return ctx.Err()
	}, nil)
	t.Cleanup(m.close)
	first, err := m.start(SNIScanRequest{Mode: "candidates"})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	ids := []string{first.ID}
	for i := 0; i < 3; i++ {
		view, err := m.start(SNIScanRequest{Mode: "candidates"})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, view.ID)
	}
	canceled, err := m.get(ids[3], true)
	if err != nil || canceled.State != "canceled" {
		t.Fatalf("queued cancellation: %+v %v", canceled, err)
	}
	next, err := m.start(SNIScanRequest{Mode: "candidates"})
	if err != nil {
		t.Fatalf("canceled job retained its queue slot: %v", err)
	}
	ids = append(ids, next.ID)
	for _, id := range ids {
		m.get(id, true)
	}
	for _, id := range ids {
		if view := waitSNIJob(t, m, id); view.State != "canceled" {
			t.Fatal(view.State)
		}
	}
}
func TestSNIRetainedSnapshotsDoNotBlockFutureScans(t *testing.T) {
	m := newSNIJobManager(func(context.Context, *sniJob) error { return nil }, nil)
	t.Cleanup(m.close)
	var first string
	for i := 0; i < 70; i++ {
		view, err := m.start(SNIScanRequest{Mode: "candidates"})
		if err != nil {
			t.Fatalf("scan %d: %v", i, err)
		}
		if i == 0 {
			first = view.ID
		}
		waitSNIJob(t, m, view.ID)
	}
	m.mu.Lock()
	count := len(m.jobs)
	m.mu.Unlock()
	if count != 64 {
		t.Fatal(count)
	}
	if _, err := m.get(first, false); err == nil {
		t.Fatal("oldest completed snapshot was not evicted")
	}
}
func TestSNIPhaseTimeoutRetainsPartialResultsAndContinuesCombinedScan(t *testing.T) {
	j := &sniJob{request: SNIScanRequest{Mode: "combined", Purpose: "reality"}}
	ctx := context.WithValue(context.Background(), sniProgressKey{}, sniProgress(j.publish))
	near := func(ctx context.Context, _ string) ([]*RealityScanResult, error) {
		emitSNIProgress(ctx, 2, 1, &RealityScanResult{Target: "8.8.8.7:443", IP: "8.8.8.7", Host: "near.example", CertValid: true, Feasible: true})
		<-ctx.Done()
		return nil, ctx.Err()
	}
	candidatesCalled := false
	candidates := func(ctx context.Context, _ string) ([]*RealityScanResult, error) {
		candidatesCalled = true
		if ctx.Err() != nil {
			t.Error("phase inherited an expired deadline")
		}
		return []*RealityScanResult{{Target: "edge.example:443", Host: "edge.example", CertValid: true}}, nil
	}
	rows, err := scanSNICombined(ctx, j, near, candidates, 5*time.Millisecond)
	if err != nil || len(rows) != 2 || !candidatesCalled || !strings.Contains(j.snapshot().Error, "partial") {
		t.Fatalf("rows=%d called=%v err=%v warning=%s", len(rows), candidatesCalled, err, j.snapshot().Error)
	}
}
func TestSNICombinedRespectsExplicitCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	j := &sniJob{}
	_, err := scanSNICombined(ctx, j, func(ctx context.Context, _ string) ([]*RealityScanResult, error) { cancel(); return nil, ctx.Err() }, func(context.Context, string) ([]*RealityScanResult, error) {
		t.Error("continued after user cancellation")
		return nil, nil
	}, time.Second)
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
func TestSNIStabilityRefreshesCertificateAndDiagnostics(t *testing.T) {
	old := true
	row := &RealityScanResult{IP: "8.8.8.8", Host: "edge.example", Port: 443, Target: "8.8.8.8:443", CertValid: true, Feasible: true, CertChainBytes: 5000, CertIssuer: "old issuer", HTTP2OK: &old, DNSLookupError: "old lookup error", TCPMs: 10, TLSMs: 20}
	index := 0
	out := measureSNICandidate(context.Background(), row, "reality", func(context.Context, string, int, string) *RealityScanResult {
		index++
		size := 4500
		if index == 2 {
			size = 2500
		}
		var check *bool
		if index == 5 {
			ok := false
			check = &ok
		}
		return &RealityScanResult{CertValid: true, Feasible: true, CertChainBytes: size, CertIssuer: "new issuer", TCPMs: 2, TLSMs: 3, HTTP2OK: check}
	}, func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
	})
	if out.CertChainBytes != 2500 || out.CertIssuer != "new issuer" || out.HTTP2OK == nil || *out.HTTP2OK || out.DNSLookupError != "" || out.Successes != 5 {
		t.Fatalf("stale certificate/diagnostics: %+v", out)
	}
	failed := measureSNICandidate(context.Background(), row, "reality", func(context.Context, string, int, string) *RealityScanResult {
		return &RealityScanResult{Reason: "TLS failed"}
	}, func(context.Context, string) ([]net.IPAddr, error) { return nil, nil })
	if failed.CertValid || failed.Feasible || failed.Successes != 0 || failed.TCPMs != 0 || failed.TLSMs != 0 || failed.HTTP2OK != nil || failed.DNSLookupError != "" || !strings.Contains(failed.Reason, "0/5") {
		t.Fatalf("stale successful measurements: %+v", failed)
	}
}
func TestSNIRankingHasConsistentOrderForUnusableResults(t *testing.T) {
	a := &RealityScanResult{Target: "a", Attempts: 5, Successes: 3, MedianMs: 100}
	b := &RealityScanResult{Target: "b", LatencyMs: 50}
	c := &RealityScanResult{Target: "c", Attempts: 5, Successes: 2, MedianMs: 1}
	for _, rows := range [][]*RealityScanResult{{a, b, c}, {a, c, b}, {b, a, c}, {b, c, a}, {c, a, b}, {c, b, a}} {
		rankSNIResults(rows, "reality")
		if rows[0] != a || rows[1] != c || rows[2] != b {
			t.Fatalf("inconsistent order: %s %s %s", rows[0].Target, rows[1].Target, rows[2].Target)
		}
	}
}
