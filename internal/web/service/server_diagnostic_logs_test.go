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

func TestDiagnosticClassifiesUnlabeledCoreFailures(t *testing.T) {
    for _, line := range []string{
        "TLS handshake: invalid sudoku hint tuple",
        "tls: bad certificate",
        "Failed to start x-ui.service",
        "panic: runtime error",
    } {
        if got := diagnosticLevel(line); got != "ERROR" {
            t.Errorf("missing severity for %q: got %q", line, got)
        }
    }
    if got := diagnosticLevel("Connection established"); got != "UNKNOWN" {
        t.Fatalf("unlabeled success = %q, want UNKNOWN", got)
    }
}

func TestDiagnosticExactSeverityFiltersDoNotIncludeOtherLevels(t *testing.T) {
    dir := t.TempDir()
    t.Setenv("XUI_LOG_FOLDER", dir)
    lines := []string{
        "2026/11/01 11:00:01 INFO - exact-filter-unique normal operation",
        "2026/11/01 11:00:02 WARNING - exact-filter-unique retrying",
        "2026/11/01 11:00:03 ERROR - exact-filter-unique certificate failed",
        "2026/11/01 11:00:04 FATAL - exact-filter-unique process crashed",
        "2026/11/01 11:00:05 NOTICE - exact-filter-unique state changed",
        "2026/11/01 11:00:06 DEBUG - exact-filter-unique detail",
        "2026/11/01 11:00:07 unclassified exact-filter-unique message",
    }
    if err := os.WriteFile(filepath.Join(dir, logger.LogFileName), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
        t.Fatal(err)
    }
    srv := &ServerService{}
    for _, tc := range []struct {
        exact string
        expected int
        wantLevel string
    }{
        {"ERROR", 1, "ERROR"},
        {"WARNING", 1, "WARNING"},
        {"FATAL", 1, "FATAL"},
        {"INFO", 1, "INFO"},
        {"NOTICE", 1, "NOTICE"},
        {"DEBUG", 1, "DEBUG"},
        {"UNKNOWN", 1, "UNKNOWN"},
        {"all", 7, ""},
    } {
        t.Run(tc.exact, func(t *testing.T) {
            got := srv.GetDashboardLogsFiltered("200", "debug", "panel", "exact-filter-unique", tc.exact)
            if len(got.Entries) != tc.expected {
                t.Fatalf("exact=%q returned %d entries; want %d: %+v", tc.exact, len(got.Entries), tc.expected, got)
            }
            for _, entry := range got.Entries {
                if tc.wantLevel != "" && entry.Level != tc.wantLevel {
                    t.Fatalf("exact=%q returned unexpected level %q", tc.exact, entry.Level)
                }
            }
        })
    }
    // Legacy threshold behavior stays unchanged for API consumers without
    // exactLevel, and specifically includes critical, error, warning levels.
    legacy := srv.GetDashboardLogs("200", "warning", "panel", "exact-filter-unique")
    if len(legacy.Entries) != 3 {
        t.Fatalf("legacy warning threshold returned %d records; want 3", len(legacy.Entries))
    }
}

func TestDiagnosticPanelWarnsWhenPersistentLogIsUnavailable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", dir)

	out := (&ServerService{}).GetDashboardLogs("10", "debug", "panel", "missing-panel-log-check")
	if len(out.Warnings) != 1 || !strings.Contains(out.Warnings[0], "panel: файл журнала отсутствует") {
		t.Fatalf("missing panel file must be reported, got %+v", out.Warnings)
	}
}

func TestDiagnosticProtocolSourceCountsDirectoriesNotFiles(t *testing.T) {
	binDir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", binDir)
	root := filepath.Join(binDir, "protocol-servers")
	if err := os.MkdirAll(filepath.Join(root, "zz-live"), 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 64; i++ {
		path := filepath.Join(root, fmt.Sprintf("a-%02d.tmp", i))
		if err := os.WriteFile(path, []byte("not a protocol directory"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	const raw = "2026/11/01 11:30:00 ERROR - audit-protocol-visibility-marker"
	if err := os.WriteFile(filepath.Join(root, "zz-live", "server.log"), []byte(raw+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out := (&ServerService{}).GetDashboardLogsFiltered("10", "debug", "protocol", "audit-protocol-visibility-marker", "ERROR")
	if len(out.Entries) != 1 || out.Entries[0].Raw != raw || out.Entries[0].Source != "protocol/zz-live" {
		t.Fatalf("unrelated files hid the active protocol log: %+v", out)
	}
	if len(out.Warnings) != 0 {
		t.Fatalf("a single protocol directory must not trigger a 64-directory warning: %+v", out.Warnings)
	}
}

func TestDiagnosticTimePreservesExplicitTimezone(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{"2026-11-01T12:00:00Z panel error", "2026-11-01T12:00:00Z"},
		{"2026-11-01T13:00:00+02:00 protocol error", "2026-11-01T11:00:00Z"},
		{"2026-11-01T13:00:00.345+0200 journal error", "2026-11-01T11:00:00.345Z"},
		{"2026-11-01T13:00:00.123456789+02:00 core error", "2026-11-01T11:00:00.123456789Z"},
	}
	for _, tc := range cases {
		got := diagnosticTime(tc.raw)
		if got.IsZero() || got.UTC().Format(time.RFC3339Nano) != tc.want {
			t.Errorf("diagnosticTime(%q) = %v; want %s", tc.raw, got, tc.want)
		}
	}
}

func TestDiagnosticProtocolLogsSortedAcrossTimezones(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	logDir := filepath.Join(dir, "protocol-servers", "test-zone")
	if err := os.MkdirAll(logDir, 0o700); err != nil { t.Fatal(err) }
	lines := []string{
		"2026-11-01T12:00:00Z INFO - zone-order-newer",
		"2026-11-01T13:00:00+02:00 INFO - zone-order-older",
	}
	if err := os.WriteFile(filepath.Join(logDir, "server.log"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := (&ServerService{}).GetDashboardLogs("10", "debug", "protocol", "zone-order")
	if len(got.Entries) != 2 || got.Entries[0].Raw != lines[0] || got.Entries[1].Raw != lines[1] {
		t.Fatalf("logs sorted by wall clock instead of absolute timestamp: %+v", got)
	}
}

func TestDiagnosticProtocolLogsKeepSubsecondOrdering(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	logDir := filepath.Join(dir, "protocol-servers", "test-precision")
	if err := os.MkdirAll(logDir, 0o700); err != nil { t.Fatal(err) }
	lines := []string{
		"2026-11-01T12:00:00.900Z INFO - precision-newer",
		"2026-11-01T12:00:00.100Z INFO - precision-older",
	}
	if err := os.WriteFile(filepath.Join(logDir, "server.log"), []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := (&ServerService{}).GetDashboardLogs("10", "debug", "protocol", "precision-")
	if len(got.Entries) != 2 || got.Entries[0].Raw != lines[0] || got.Entries[1].Raw != lines[1] {
		t.Fatalf("subsecond sorting lost chronological order: %+v", got)
	}
	if got.Entries[0].Time != "2026-11-01T12:00:00.9Z" || got.Entries[1].Time != "2026-11-01T12:00:00.1Z" {
		t.Fatalf("subsecond timestamps were truncated: %+v", got.Entries)
	}
}

func TestDiagnosticPanelPreservesShortPanicMessage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", dir)
	const line = "panic: SIGSEGV"
	if err := os.WriteFile(filepath.Join(dir, logger.LogFileName), []byte(line+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := (&ServerService{}).GetDashboardLogsFiltered("10", "debug", "panel", "SIGSEGV", "ERROR")
	if len(got.Entries) != 1 || got.Entries[0].Raw != line || got.Entries[0].Level != "ERROR" {
		t.Fatalf("short panic entry lost from structured diagnostics: %+v", got)
	}
}

func TestDiagnosticTimeIgnoresTimestampInsideMessage(t *testing.T) {
	line := "2026/11/01 10:00:00 INFO - certificate renews at 2027-01-01T00:00:00Z"
	got := diagnosticTime(line)
	want, err := time.ParseInLocation("2006/01/02 15:04:05", "2026/11/01 10:00:00", time.Local)
	if err != nil { t.Fatal(err) }
	if !got.Equal(want) {
		t.Fatalf("message date replaced the log timestamp: got %v, want %v", got, want)
	}
}
