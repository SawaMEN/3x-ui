package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFail2banProbeDoesNotBlockStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires a local Unix shell fixture")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "pid")
	script := filepath.Join(dir, "fail2ban-client")
	body := "#!/bin/sh\nprintf '%d' \"$$\" > \"$XUI_TEST_FAIL2BAN_PID\"\nexec /bin/sleep 30\n"
	if err := os.WriteFile(script, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("XUI_TEST_FAIL2BAN_PID", marker)
	t.Setenv("XUI_ENABLE_FAIL2BAN", "true")
	type result struct {
		state fail2banState
		err   error
	}
	done := make(chan result, 1)
	go func() { state, err := fail2banEnforcementState(); done <- result{state, err} }()
	// Clean up even on the original unbounded implementation.
	finished := false
	t.Cleanup(func() {
		if finished {
			return
		}
		if data, err := os.ReadFile(marker); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil {
				if process, err := os.FindProcess(pid); err == nil {
					_ = process.Kill()
				}
			}
		}
	})
	select {
	case r := <-done:
		finished = true
		if r.state != fail2banUnknown || !errors.Is(r.err, context.DeadlineExceeded) {
			t.Fatalf("stalled probe must preserve limits as unknown: %v %v", r.state, r.err)
		}
	case <-time.After(4 * time.Second):
		t.Fatal("stalled fail2ban-client blocked startup past the probe budget")
	}
}
