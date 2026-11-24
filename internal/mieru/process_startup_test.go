package mieru

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMitaEarlyExitIsNotSuccessfulStartup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	for _, status := range []string{"0", "1"} {
		if err := os.WriteFile(filepath.Join(dir, GetBinaryName()), []byte("#!/bin/sh\nexit "+status+"\n"), 0o700); err != nil {
			t.Fatal(err)
		}
		p := newProcess(filepath.Join(dir, "config.json"), "startup-test", filepath.Join(dir, "mita.sock"))
		if err := p.Start(); err == nil || !strings.Contains(err.Error(), "exited during startup") {
			t.Fatalf("exit %s: expected startup error, got %v", status, err)
		}
		if p.IsRunning() {
			t.Fatal("exited process is marked running")
		}
	}
}
