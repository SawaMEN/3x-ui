package service

import (
	"context"
	"errors"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"net"
	"path/filepath"
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
	close(m.queue)
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
	close(m.queue)
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
	close(m.queue)
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
