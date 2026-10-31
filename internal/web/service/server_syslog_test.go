//go:build !windows

package service

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// A journalctl that never answers must not hang the Syslog request (#6629).
func TestGetLogsSyslogGivesUpOnAStalledJournalctl(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte("#!/bin/sh\nexec sleep 5\n"), 0o755); err != nil {
		t.Fatalf("write fake journalctl: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	saved := syslogTimeout
	syslogTimeout = 200 * time.Millisecond
	t.Cleanup(func() { syslogTimeout = saved })

	start := time.Now()
	got := (&ServerService{}).GetLogs("10", "err", "true")
	want := []string{"journalctl did not answer in time. Try a smaller line count or a less strict level."}
	if !slices.Equal(got, want) {
		t.Fatalf("GetLogs = %q, want %q", got, want)
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("GetLogs waited %v for a stalled journalctl, want about %v", elapsed, syslogTimeout)
	}
}

func TestGetLogsSyslogReturnsNewestFirstWithoutBlankLine(t *testing.T) {
    dir := t.TempDir()
    fake := "#!/bin/sh\nprintf 'old entry\\nnew entry\\n'\n"
    if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte(fake), 0o755); err != nil {
        t.Fatal(err)
    }
    t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
    got := (&ServerService{}).GetLogs("10", "debug", "true")
    want := []string{"new entry", "old entry"}
    if !slices.Equal(got, want) {
        t.Fatalf("syslog order / trailing blank row: got %#v, want %#v", got, want)
    }
}

func TestGetLogsSyslogHandlesEmptyOutput(t *testing.T) {
    dir := t.TempDir()
    if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
        t.Fatal(err)
    }
    t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
    if got := (&ServerService{}).GetLogs("10", "debug", "true"); len(got) != 0 {
        t.Fatalf("empty syslog must not create a blank entry: %#v", got)
    }
}

func TestGetLogsSyslogStopsWaitingForInheritedPipe(t *testing.T) {
    dir := t.TempDir()
    // The parent exits immediately, but its background child retains stdout.
    // Without WaitDelay, Cmd.Run can wait for this child despite a short
    // journalctl command and a bounded context timeout.
    script := "#!/bin/sh\nsleep 5 &\nprintf 'recent entry\\n'\n"
    if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte(script), 0o755); err != nil {
        t.Fatal(err)
    }
    t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
    previous := syslogWaitDelay
    syslogWaitDelay = 120 * time.Millisecond
    t.Cleanup(func() { syslogWaitDelay = previous })

    started := time.Now()
    got := (&ServerService{}).GetLogs("10", "debug", "true")
    if elapsed := time.Since(started); elapsed > 2*time.Second {
        t.Fatalf("legacy syslog waited %s for an orphan stdout writer", elapsed)
    }
    if len(got) == 0 {
        t.Fatal("legacy syslog did not respond")
    }
}

func TestDiagnosticJournalStopsWaitingForInheritedPipe(t *testing.T) {
    dir := t.TempDir()
    script := "#!/bin/sh\nsleep 5 &\nprintf '2026-11-01T12:00:00Z ERROR - journal-orphan-marker\\n'\n"
    if err := os.WriteFile(filepath.Join(dir, "journalctl"), []byte(script), 0o755); err != nil {
        t.Fatal(err)
    }
    t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
    previous := syslogWaitDelay
    syslogWaitDelay = 120 * time.Millisecond
    t.Cleanup(func() { syslogWaitDelay = previous })

    started := time.Now()
    got := (&ServerService{}).GetDashboardLogsFiltered("10", "debug", "journal", "journal-orphan-marker", "ERROR")
    if elapsed := time.Since(started); elapsed > 2*time.Second {
        t.Fatalf("structured journal waited %s for an orphan stdout writer", elapsed)
    }
    if len(got.Warnings) == 0 && len(got.Entries) == 0 {
        t.Fatalf("diagnostics silently lost orphaned journal status: %+v", got)
    }
}
