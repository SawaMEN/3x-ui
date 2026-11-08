package service

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestTelemtUpdaterRepairsNonExecutableInstallation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Telemt updater is a Linux shell script")
	}
	path := filepath.Join(t.TempDir(), "telemt-update.sh")
	if err := os.WriteFile(path, []byte("#!/bin/bash\necho latest=1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// The installed script may have lost its executable bit, but checking the
	// version must still work and the next update repairs the file mode.
	out, err := telemtUpdaterCommand(context.Background(), path, "--check").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "latest=1.2.3") {
		t.Fatalf("check a non-executable updater: %s, %v", out, err)
	}
	if err := repairTelemtUpdaterMode(path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("updater mode = %v, want 0755", info.Mode().Perm())
	}
}

func TestTelemtUpdaterCancellationAllowsRollbackTrap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Telemt updater is a Linux shell script")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "recovered")
	path := filepath.Join(dir, "update.sh")
	script := "trap 'echo recovered > \"$1\"; exit 130' INT\necho ready\nwhile :; do :; done\n"
	if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := telemtUpdaterCommand(ctx, path, marker)
	// A broken cancellation implementation must not leave a busy test child.
	cmd.WaitDelay = time.Second
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	if ready, err := bufio.NewReader(out).ReadString('\n'); err != nil || ready != "ready\n" {
		t.Fatalf("updater failed to arm trap: %q: %v", ready, err)
	}
	cancel()
	if err := cmd.Wait(); err == nil {
		t.Fatal("cancelled update unexpectedly succeeded")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != "recovered\n" {
		t.Fatalf("cancellation bypassed rollback trap: %q: %v", data, err)
	}
}
