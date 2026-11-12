package externalvpn

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestTrafficCheckedKeepsHealthySamplesAndFailedBaseline(t *testing.T) {
	var failed atomic.Bool
	failed.Store(true)
	var total atomic.Uint64
	total.Store(10)
	healthy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `[{"username":"healthy","inbound":%d,"outbound":0}]`, total.Load())
	}))
	defer healthy.Close()
	recovering := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failed.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("private-body-token"))
			return
		}
		_, _ = w.Write([]byte(`[{"username":"recovering","inbound":35,"outbound":0}]`))
	}))
	defer recovering.Close()
	good := &running{protocol: model.TrustTunnel, instance: Instance{ID: 1}, tag: "good", metricsAddr: strings.TrimPrefix(healthy.URL, "http://"), counters: map[string]trafficCounters{}}
	bad := &running{protocol: model.TrustTunnel, instance: Instance{ID: 2}, tag: "bad", metricsAddr: strings.TrimPrefix(recovering.URL, "http://"), counters: map[string]trafficCounters{"recovering": {up: 7}}}
	mgr := &Manager{procs: map[int]*running{1: good, 2: bad}}
	rows, err := mgr.CollectTrafficChecked()
	if err == nil || len(rows) != 1 || rows[0].Email != "healthy" || rows[0].Up != 10 {
		t.Fatalf("partial sample: %+v err=%v", rows, err)
	}
	if strings.Contains(err.Error(), "private-body-token") || strings.Contains(err.Error(), recovering.URL) {
		t.Fatal("metrics error exposes private response or URL")
	}
	if bad.counters["recovering"].up != 7 {
		t.Fatal("failed poll advanced cumulative baseline")
	}
	failed.Store(false)
	total.Store(15)
	rows, err = mgr.CollectTrafficChecked()
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, row := range rows {
		got[row.Email] = row.Up
	}
	if got["healthy"] != 5 || got["recovering"] != 28 {
		t.Fatalf("recovery deltas: %+v", got)
	}
}

func TestTrafficCheckedRejectsInvalidAndOversizeResponses(t *testing.T) {
	for _, body := range []string{"null", "{bad", `[{"username":"x","inbound":1}] trailing`, strings.Repeat(" ", (1<<20)+1)} {
		t.Run(fmt.Sprint(len(body)), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
			defer srv.Close()
			proc := &running{protocol: model.TrustTunnel, metricsAddr: strings.TrimPrefix(srv.URL, "http://"), counters: map[string]trafficCounters{"x": {up: 9}}}
			if _, err := (&Manager{}).collectProcessTrafficChecked(proc); err == nil {
				t.Fatal("invalid metrics accepted")
			}
			if proc.counters["x"].up != 9 {
				t.Fatal("invalid response changed baseline")
			}
		})
	}
}

func TestAdditionalTrafficCheckedSanitizesFailures(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	for _, protocol := range []model.Protocol{model.FPTN, model.OpenFlux} {
		proc := &running{protocol: protocol, instance: Instance{ID: 9, Settings: Settings{Certificate: "missing-private-cert", MetricsKey: "private-metrics-key"}}, counters: map[string]trafficCounters{}}
		if _, err := (&Manager{}).collectAdditionalTrafficChecked(proc); err == nil || strings.Contains(err.Error(), "private-") {
			t.Fatalf("unsanitized/hidden failure for %s: %v", protocol, err)
		}
	}
	// Pingtunnel has no per-client API; its lack of metrics is not an API outage.
	if _, err := (&Manager{}).collectProcessTrafficChecked(&running{protocol: model.Pingtunnel}); err != nil {
		t.Fatal(err)
	}
}
