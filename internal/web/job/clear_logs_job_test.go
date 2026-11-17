package job

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
)

func writeLogConfig(t *testing.T, accessPath string, errorPath string) {
	t.Helper()
	binDir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", binDir)
	configData, err := json.Marshal(map[string]any{
		"log": map[string]any{"access": accessPath, "error": errorPath},
	})
	if err != nil {
		t.Fatalf("marshal xray config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "config.json"), configData, 0o644); err != nil {
		t.Fatalf("write xray config: %v", err)
	}
}

func TestPruneCurrentCoreAndIPLogs(t *testing.T) {
	oldMax := maxXrayLogBytes
	maxXrayLogBytes = 8
	t.Cleanup(func() { maxXrayLogBytes = oldMax })
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_LOG_FOLDER", dir)
	if err := os.MkdirAll(filepath.Dir(hiddify.GetConfigPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	// No legacy config.json exists. A relative output uses the core working directory.
	if err := os.WriteFile(hiddify.GetConfigPath(), []byte(`{"log":{"output":"custom.log"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	coreLog := filepath.Join(filepath.Dir(hiddify.GetBinaryPath()), "custom.log")
	paths := []string{coreLog, filepath.Join(dir, "3xipl.log"), filepath.Join(dir, "3xipl-banned.log"), filepath.Join(dir, "3xipl-banned.prev.log")}
	for _, path := range paths {
		if err := os.WriteFile(path, []byte("oversized log line"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Truncation must preserve an open writer's inode.
	writer, err := os.OpenFile(coreLog, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	NewPruneXrayLogsJob().Run()
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || info.Size() != 0 {
			t.Fatalf("log not pruned: %s (%v)", path, err)
		}
	}
	if _, err := writer.WriteString("new"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(coreLog)
	if err != nil || string(data) != "new" {
		t.Fatalf("open writer lost after pruning: %q, %v", data, err)
	}
}

func TestProtocolServerLogsArePrunedAndCleared(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_LOG_FOLDER", dir)
	oldMax := maxXrayLogBytes
	maxXrayLogBytes = 8
	t.Cleanup(func() { maxXrayLogBytes = oldMax })
	path := filepath.Join(dir, "protocol-servers", "standalone", "server.log")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	writer, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	if _, err := writer.WriteString("oversized server log"); err != nil {
		t.Fatal(err)
	}
	NewPruneXrayLogsJob().Run()
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Fatalf("standalone log not pruned: %v", err)
	}
	if _, err := writer.WriteString("new"); err != nil {
		t.Fatal(err)
	}
	NewPruneXrayLogsJob().Run()
	if data, err := os.ReadFile(path); err != nil || string(data) != "new" {
		t.Fatalf("small log or open writer changed: %q, %v", data, err)
	}
	NewClearLogsJob().Run()
	if info, err := os.Stat(path); err != nil || info.Size() != 0 {
		t.Fatalf("standalone log not cleared: %v", err)
	}
}

func TestCoreLogDisabledAndDailyCleanup(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	path := filepath.Join(dir, "core.log")
	if err := os.MkdirAll(filepath.Dir(hiddify.GetConfigPath()), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, disabled := range []bool{true, false} {
		cfg, _ := json.Marshal(map[string]any{"log": map[string]any{"disabled": disabled, "output": path}})
		if err := os.WriteFile(hiddify.GetConfigPath(), cfg, 0o600); err != nil {
			t.Fatal(err)
		}
		wipeXrayLogs()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if disabled && string(data) != "keep" || !disabled && len(data) != 0 {
			t.Fatalf("disabled=%v: unexpected log content %q", disabled, data)
		}
	}
}

func TestDailyBanSnapshotBounds(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_LOG_FOLDER", dir)
	t.Setenv("XUI_BIN_FOLDER", dir)
	oldMax := maxXrayLogBytes
	maxXrayLogBytes = 8
	t.Cleanup(func() { maxXrayLogBytes = oldMax })
	path := filepath.Join(dir, "3xipl-banned.log")
	if err := os.WriteFile(path, []byte("old entries\nnew tail"), 0o600); err != nil {
		t.Fatal(err)
	}
	NewClearLogsJob().Run()
	data, err := os.ReadFile(filepath.Join(dir, "3xipl-banned.prev.log"))
	if err != nil || string(data) != "new tail" {
		t.Fatalf("snapshot exceeds bounded tail: %q, %v", data, err)
	}
	data, err = os.ReadFile(path)
	if err != nil || len(data) != 0 {
		t.Fatalf("source not cleared: %q, %v", data, err)
	}
}

func TestWipeXrayLogs_TruncatesEnabledLogs(t *testing.T) {
	accessLog := filepath.Join(t.TempDir(), "access.log")
	errorLog := filepath.Join(t.TempDir(), "error.log")
	if err := os.WriteFile(accessLog, []byte("2026/06/23 12:00:00 from tcp:203.0.113.10:443 accepted\n"), 0o644); err != nil {
		t.Fatalf("seed access log: %v", err)
	}
	if err := os.WriteFile(errorLog, []byte("xray warning\n"), 0o644); err != nil {
		t.Fatalf("seed error log: %v", err)
	}
	writeLogConfig(t, accessLog, errorLog)

	wipeXrayLogs()

	for _, logPath := range []string{accessLog, errorLog} {
		info, err := os.Stat(logPath)
		if err != nil {
			t.Fatalf("%s should still exist: %v", logPath, err)
		}
		if info.Size() != 0 {
			t.Fatalf("%s should be truncated to 0, got %d bytes", logPath, info.Size())
		}
	}
}

func TestWipeXrayLogs_LeavesDisabledLogsAlone(t *testing.T) {
	writeLogConfig(t, "none", "none")

	wipeXrayLogs()

	if _, err := os.Stat("none"); err == nil {
		os.Remove("none")
		t.Fatal(`wipeXrayLogs must not create a file named "none"`)
	}
}

func TestPruneXrayLogs_TruncatesOnlyOversizedLogs(t *testing.T) {
	oldMax := maxXrayLogBytes
	maxXrayLogBytes = 8
	defer func() { maxXrayLogBytes = oldMax }()

	dir := t.TempDir()
	accessLog := filepath.Join(dir, "access.log")
	errorLog := filepath.Join(dir, "error.log")
	if err := os.WriteFile(accessLog, []byte("small"), 0o644); err != nil {
		t.Fatalf("seed access log: %v", err)
	}
	if err := os.WriteFile(errorLog, []byte("large log line"), 0o644); err != nil {
		t.Fatalf("seed error log: %v", err)
	}
	writeLogConfig(t, accessLog, errorLog)

	NewPruneXrayLogsJob().Run()

	accessInfo, err := os.Stat(accessLog)
	if err != nil {
		t.Fatalf("access log should still exist: %v", err)
	}
	if accessInfo.Size() != 5 {
		t.Fatalf("small access log should be left alone, got %d bytes", accessInfo.Size())
	}
	errorInfo, err := os.Stat(errorLog)
	if err != nil {
		t.Fatalf("error log should still exist: %v", err)
	}
	if errorInfo.Size() != 0 {
		t.Fatalf("oversized error log should be truncated, got %d bytes", errorInfo.Size())
	}
}
