//go:build !windows

package mieru

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTrafficCheckedRejectsReplacedProcessSnapshot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	entered, release := filepath.Join(dir, "entered"), filepath.Join(dir, "release")
	t.Setenv("XUI_AUDIT_ENTERED", entered)
	t.Setenv("XUI_AUDIT_RELEASE", release)
	script := []byte("#!/bin/sh\nif [ \"$1\" = get ]; then\n: > \"$XUI_AUDIT_ENTERED\"\nwhile [ ! -f \"$XUI_AUDIT_RELEASE\" ]; do sleep 0.01; done\necho 'synthetic - 220B 110B'\nelse exec sleep 60; fi\n")
	if err := os.WriteFile(filepath.Join(dir, "mita"), script, 0o700); err != nil {
		t.Fatal(err)
	}
	mgr := &Manager{procs: map[int]*managed{}, lastErr: map[int]string{}, traffic: map[int]map[string]trafficCursor{}}
	inst := Instance{Id: 1, Tag: "old", Users: []User{{Name: "synthetic", Password: "test"}}, PortBindings: []PortBinding{{Port: 34573, Protocol: "TCP"}}}
	if err := mgr.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	old := mgr.procs[1]
	defer old.proc.Stop()
	mgr.traffic[1] = map[string]trafficCursor{"synthetic": {up: 10, down: 20, initialized: true}}
	type result struct {
		rows []TrafficDelta
		err  error
	}
	done := make(chan result, 1)
	go func() {
		rows, _, err := mgr.CollectTrafficChecked([]Instance{inst})
		done <- result{rows: rows, err: err}
	}()
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(entered); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("poll did not enter fake metrics command")
		}
		time.Sleep(time.Millisecond)
	}
	mgr.mu.Lock()
	replacement := &managed{tag: "new"}
	mgr.procs[1] = replacement
	mgr.traffic[1] = map[string]trafficCursor{"synthetic": {up: 3, down: 4, initialized: true}}
	mgr.mu.Unlock()
	if err := os.WriteFile(release, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case got := <-done:
		if len(got.rows) != 0 || got.err == nil {
			t.Fatalf("process replacement: rows=%+v err=%v", got.rows, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("poll did not return")
	}
	mgr.mu.Lock()
	cursor := mgr.traffic[1]["synthetic"]
	mgr.mu.Unlock()
	if cursor.up != 3 || cursor.down != 4 {
		t.Fatalf("old process corrupted new cursor: %+v", cursor)
	}
}

func TestTrafficCheckedKeepsCursorAfterCommandFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	mode := filepath.Join(dir, "fail")
	t.Setenv("XUI_AUDIT_FAIL_FILE", mode)
	script := []byte("#!/bin/sh\nif [ \"$1\" = get ]; then\nif [ -f \"$XUI_AUDIT_FAIL_FILE\" ]; then exit 1; fi\necho 'synthetic - 220B 110B'\nelse exec sleep 60; fi\n")
	if err := os.WriteFile(filepath.Join(dir, "mita"), script, 0o700); err != nil {
		t.Fatal(err)
	}
	mgr := &Manager{procs: map[int]*managed{}, lastErr: map[int]string{}, traffic: map[int]map[string]trafficCursor{}}
	inst := Instance{Id: 1, Tag: "test", Users: []User{{Name: "synthetic", Password: "test"}}, PortBindings: []PortBinding{{Port: 34573, Protocol: "TCP"}}}
	if err := mgr.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	defer mgr.StopAll()
	mgr.traffic[1] = map[string]trafficCursor{"synthetic": {up: 10, down: 20, initialized: true}}
	if err := os.WriteFile(mode, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := mgr.CollectTrafficChecked([]Instance{inst}); err == nil {
		t.Fatal("command failure hidden")
	}
	if cursor := mgr.traffic[1]["synthetic"]; cursor.up != 10 || cursor.down != 20 {
		t.Fatal("failed command moved cursor")
	}
	if err := os.Remove(mode); err != nil {
		t.Fatal(err)
	}
	rows, _, err := mgr.CollectTrafficChecked([]Instance{inst})
	if err != nil || len(rows) != 1 || rows[0].Up != 100 || rows[0].Down != 200 {
		t.Fatalf("recovery sample: %+v err=%v", rows, err)
	}
}

func TestScrapeUsersCheckedDistinguishesEmptyTableAndInvalidOutput(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	output := filepath.Join(dir, "output")
	t.Setenv("XUI_AUDIT_OUTPUT", output)
	if err := os.WriteFile(filepath.Join(dir, "mita"), []byte("#!/bin/sh\ncat \"$XUI_AUDIT_OUTPUT\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"", "upstream diagnostic without a table", "synthetic - " + strings.Repeat("9", 400) + "B 1B"} {
		if err := os.WriteFile(output, []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := scrapeUsersChecked("unused-socket", "unused-config"); err == nil {
			t.Fatalf("invalid output accepted: %q", value)
		}
	}
	if err := os.WriteFile(output, []byte("User LastActive 1DayDown 1DayUp 7DaysDown 7DaysUp\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if rows, err := scrapeUsersChecked("unused-socket", "unused-config"); err != nil || len(rows) != 0 {
		t.Fatalf("empty table: %+v %v", rows, err)
	}
}
