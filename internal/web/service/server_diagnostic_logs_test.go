package service

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
    "testing"
    "time"

    "github.com/SawaMEN/3x-ui/v3/internal/logger"
)

func TestDiagnosticPanelIncludesAllLevelsAndFullRaw(t *testing.T) {
    dir := t.TempDir()
    t.Setenv("XUI_LOG_FOLDER", dir)
    lines := []string{
        "2026/11/01 01:20:00 INFO - diag-unique connected",
        "2026/11/01 01:20:01 WARNING - diag-unique retrying connection",
        "2026/11/01 01:20:02 ERROR - diag-unique nested TLS failure: bad certificate",
        "2026/11/01 01:20:03 DEBUG - diag-unique packet details",
    }
    if err := os.WriteFile(filepath.Join(dir, logger.LogFileName), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil { t.Fatal(err) }

    out := (&ServerService{}).GetDashboardLogs("50", "debug", "panel", "diag-unique")
    if len(out.Entries) != 4 || len(out.Warnings) != 0 { t.Fatalf("all entries: %+v", out) }
    if out.Entries[0].Raw != lines[3] || out.Entries[2].Raw != lines[1] { t.Fatalf("incorrect order/raw: %+v", out.Entries) }
    for _, e := range out.Entries {
        if e.Source != "panel" { t.Fatalf("unexpected source: %+v", e) }
        if e.Time == "" { t.Fatalf("missing timestamp: %+v", e) }
    }
    errors := (&ServerService{}).GetDashboardLogs("50", "error", "panel", "certificate")
    if len(errors.Entries) != 1 || errors.Entries[0].Raw != lines[2] {
        t.Fatalf("error filter silently dropped raw details: %+v", errors)
    }
    notice := (&ServerService{}).GetDashboardLogs("50", "notice", "panel", "diag-unique")
    if len(notice.Entries) != 2 { t.Fatalf("notice filter = %+v", notice) }
}

func TestDiagnosticLogsBoundedAndSearchesBeyondDisplayLimit(t *testing.T) {
    dir := t.TempDir()
    t.Setenv("XUI_LOG_FOLDER", dir)
    lines := make([]string, 0, 600)
    for i := 0; i < 600; i++ {
        lines = append(lines, fmt.Sprintf("2026/11/01 01:20:00 INFO - message-%03d", i))
    }
    if err := os.WriteFile(filepath.Join(dir, logger.LogFileName), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil { t.Fatal(err) }
    srv := &ServerService{}
    result := srv.GetDashboardLogs("20", "debug", "panel", "message-095")
    if len(result.Entries) != 1 || !strings.Contains(result.Entries[0].Raw, "message-095") {
        t.Fatalf("search missed older matching line: %+v", result)
    }
    result = srv.GetDashboardLogs("9999999", "debug", "panel", "message-")
    if len(result.Entries) != 600 { t.Fatalf("bounded count = %d", len(result.Entries)) }
    if dashboardLogCount("-1") != 100 || dashboardLogCount("9999999") != dashboardLogLimit {
        t.Fatal("unbounded or invalid count")
    }
}

func TestDiagnosticUnknownFormatsAreNotSilentlyDropped(t *testing.T) {
    t.Setenv("XUI_LOG_FOLDER", t.TempDir())
    raw := "uncategorized stack trace without a prefix"
    if err := os.WriteFile(filepath.Join(os.Getenv("XUI_LOG_FOLDER"), logger.LogFileName), []byte(raw+"\n"), 0o600); err != nil { t.Fatal(err) }
    out := (&ServerService{}).GetDashboardLogs("20", "debug", "panel", raw)
    if len(out.Entries) != 1 || out.Entries[0].Raw != raw || out.Entries[0].Level != "UNKNOWN" {
        t.Fatalf("lost unstructured log: %+v", out)
    }
    journalTime := diagnosticTime("2026-11-01T01:12:13+0200 host x-ui[12]: error")
    if journalTime.IsZero() || journalTime.Format(time.RFC3339) != "2026-11-01T01:12:13+02:00" {
        t.Fatalf("wrong journal timestamp: %v", journalTime)
    }
}
