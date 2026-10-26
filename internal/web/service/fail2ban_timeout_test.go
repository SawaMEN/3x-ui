//go:build !windows

package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFail2banProbeCannotBlockPanelRequests(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fail2ban-client")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	svc := &ServerService{}
	started := time.Now()
	if svc.isFail2banInstalled() {
		t.Fatal("timed out probe reported fail2ban installed")
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("probe blocked panel for %s", elapsed)
	}
	started = time.Now()
	if svc.isFail2banInstalled() {
		t.Fatal("cached failed probe reported installed")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("failed probe was not cached: %s", elapsed)
	}
}
