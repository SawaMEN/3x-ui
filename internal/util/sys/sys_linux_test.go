//go:build linux

package sys

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestCPUPercentRawCounterChanges(t *testing.T) {
	proc := t.TempDir()
	t.Setenv("HOST_PROC", proc)
	previousBusy, previousIdle, previousHasLast := lastBusy, lastIdleAll, hasLast
	t.Cleanup(func() { lastBusy, lastIdleAll, hasLast = previousBusy, previousIdle, previousHasLast })
	lastBusy, lastIdleAll, hasLast = 0, 0, false
	for _, step := range []struct {
		counters string
		want     float64
		wantErr  bool
	}{
		{"cpu 100 0 0 100 100\n", 0, false},
		{"cpu 110 0 0 105 invalid\n", 0, true},
		{"cpu 110 0 0 105 90\n", 100, false},
		{"cpu 5 0 0 5 0\n", 0, false},
		{"cpu 15 0 0 15 0\n", 50, false},
	} {
		if err := os.WriteFile(filepath.Join(proc, "stat"), []byte(step.counters), 0o600); err != nil {
			t.Fatal(err)
		}
		actual, err := CPUPercentRaw()
		if (err != nil) != step.wantErr || math.Abs(actual-step.want) > 0.001 {
			t.Errorf("stat %q: got (%v, %v), want (%v, error=%v)", step.counters, actual, err, step.want, step.wantErr)
		}
	}
}
