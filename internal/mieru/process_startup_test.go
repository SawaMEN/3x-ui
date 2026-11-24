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

func TestMitaConcurrentStartOwnsOneProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	if err := os.WriteFile(filepath.Join(dir, GetBinaryName()), []byte("#!/bin/sh\nexec sleep 30\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	p := newProcess(filepath.Join(dir, "config.json"), "concurrent", filepath.Join(dir, "mita.sock"))
	t.Cleanup(func() { _ = p.Stop() })
	start := make(chan struct{})
	results := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			results <- p.Start()
		}()
	}
	close(start)
	successes := 0
	for range 2 {
		if err := <-results; err == nil {
			successes++
		}
	}
	if successes != 1 || !p.IsRunning() {
		t.Fatalf("successful starts = %d, running = %v", successes, p.IsRunning())
	}
}
