package externalvpn

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestTrustTunnelSplitMetricsIndependentResets(t *testing.T) {
	proc := &running{tag: "tt", counters: map[string]trafficCounters{}}
	poll := func(tcp, quic uint64, active bool) TrafficDelta {
		t.Helper()
		sessions := 0
		if active {
			sessions = 1
		}
		rows := collectTrustTunnelMetrics(proc,
			[]trustTunnelMetric{{Username: "alice", Inbound: tcp, Outbound: tcp}},
			[]trustTunnelMetric{{Username: "alice", Inbound: quic, Outbound: quic, Sessions: sessions}}, true)
		if len(rows) != 1 {
			t.Fatalf("expected one client delta, got %d", len(rows))
		}
		return rows[0]
	}
	for _, tc := range []struct {
		tcp, quic uint64
		want      int64
		active    bool
	}{
		{100, 200, 300, true},
		{110, 220, 30, false},
		{120, 5, 15, true},
		{1, 10, 6, false},
		{1, 10, 0, false},
	} {
		if row := poll(tc.tcp, tc.quic, tc.active); row.Up != tc.want || row.Down != tc.want || row.Active != tc.active || row.Email != "alice" {
			t.Fatalf("incorrect merged delta: %+v", row)
		}
	}
}

func TestTrustTunnelSplitMetricsFailedPollDoesNotAdvance(t *testing.T) {
	tcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"username":"alice","inbound":100,"outbound":50}]`)
	}))
	defer tcp.Close()
	quic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer quic.Close()
	proc := &running{
		protocol: model.TrustTunnel, tag: "tt", metricsAddr: strings.TrimPrefix(tcp.URL, "http://"),
		quicMetricsAddr: strings.TrimPrefix(quic.URL, "http://"), counters: map[string]trafficCounters{"alice": {up: 10, down: 20}},
	}
	_, err := (&Manager{}).collectProcessTrafficChecked(proc)
	if err == nil || !reflect.DeepEqual(proc.counters, map[string]trafficCounters{"alice": {up: 10, down: 20}}) {
		t.Fatal("a failed QUIC poll consumed TCP counters")
	}
}

func TestTrustTunnelPairHelper(t *testing.T) {
	switch os.Getenv("TRUSTTUNNEL_PAIR_TEST_CHILD") {
	case "wait":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "exit":
		os.Exit(2)
	}
}

func trustTunnelTestChild(ctx context.Context, behavior string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestTrustTunnelPairHelper$")
	cmd.Env = append(os.Environ(), "TRUSTTUNNEL_PAIR_TEST_CHILD="+behavior)
	return cmd
}

func TestTrustTunnelPairReapsFirstOnSecondStartFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	first := trustTunnelTestChild(ctx, "wait")
	second := exec.CommandContext(ctx, t.TempDir()+"/nonexistent")
	if err := startExternalVPNChildren(first, second, cancel); err == nil {
		t.Fatal("accepted unavailable second child")
	}
	if first.ProcessState == nil {
		t.Fatal("first child was not reaped")
	}
}

func TestTrustTunnelPairStopsCompanionOnExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	first, second := trustTunnelTestChild(ctx, "exit"), trustTunnelTestChild(ctx, "wait")
	if err := startExternalVPNChildren(first, second, cancel); err != nil {
		t.Fatal(err)
	}
	if err := waitExternalVPNChildren(first, second, cancel); err == nil || first.ProcessState == nil || second.ProcessState == nil {
		t.Fatal("pair exit did not reap both children")
	}
}

func TestTrustTunnelMetricsHonorCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `[]`)
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := readTrustTunnelMetrics(ctx, strings.TrimPrefix(server.URL, "http://")); err == nil {
		t.Fatal("metrics request ignored a canceled context")
	}
}
