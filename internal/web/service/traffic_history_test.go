package service

import "testing"

func TestCounterDeltaHandlesReset(t *testing.T) {
	cases := []struct {
		name     string
		current  int64
		previous int64
		want     int64
	}{
		{name: "growth", current: 150, previous: 100, want: 50},
		{name: "same", current: 100, previous: 100, want: 0},
		{name: "reset", current: 20, previous: 100, want: 20},
		{name: "zero", current: 0, previous: 100, want: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := counterDelta(tc.current, tc.previous); got != tc.want {
				t.Fatalf("counterDelta(%d, %d) = %d, want %d", tc.current, tc.previous, got, tc.want)
			}
		})
	}
}

func TestTrafficHistoryDeltasSkipNewCounters(t *testing.T) {
	previous := map[string]trafficHistoryCounter{
		historyCounterKey("client", "alice"): {resource: "client", tag: "alice", up: 100, down: 200},
	}
	current := map[string]trafficHistoryCounter{
		historyCounterKey("client", "alice"): {resource: "client", tag: "alice", up: 120, down: 250},
		historyCounterKey("client", "bob"):   {resource: "client", tag: "bob", up: 999, down: 999},
	}
	got := trafficHistoryDeltas(previous, current)
	if len(got) != 1 {
		t.Fatalf("deltas len = %d, want 1: %#v", len(got), got)
	}
	if got[0].Tag != "alice" || got[0].Up != 20 || got[0].Down != 50 {
		t.Fatalf("delta = %#v, want alice up=20 down=50", got[0])
	}
}
