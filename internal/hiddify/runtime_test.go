package hiddify

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/core"
)

func TestRuntimeLifecycleAndInvalidCandidate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process fixture")
	}
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	script := fmt.Sprintf(`#!/bin/sh
case "$1" in
version) printf 'hiddify-core version %s hiddify-sing-box version test\nTags: %s\nRevision: %s\n' ;;
check) if [ "$2" != '-c' ] || ! test -f "$3"; then exit 2; fi; if grep -q invalid "$3"; then echo 'invalid candidate' >&2; exit 3; fi ;;
srun) exec sleep 60 ;;
*) exit 4 ;;
esac
`, TargetVersion, strings.Join(append(append([]string{}, requiredBuildTags...), "with_purego"), ","), TargetRevision)
	if err := os.WriteFile(GetBinaryPath(), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "candidate.json")
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := NewRuntime(path, true)
	var contract core.Runtime = r
	if contract.Type() != core.Hiddify {
		t.Fatal("wrong core identity")
	}
	t.Cleanup(func() { _ = r.Stop(context.Background()) })
	cancelled, cancelStart := context.WithCancel(context.Background())
	cancelStart()
	if err := contract.Start(cancelled); !errors.Is(err, context.Canceled) || contract.IsRunning() {
		t.Fatalf("cancelled start launched child: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	if err := contract.Start(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	if !contract.IsRunning() {
		t.Fatal("core died with request context")
	}
	if err := os.WriteFile(path, []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := contract.Restart(context.Background()); err == nil {
		t.Fatal("invalid candidate restarted core")
	}
	if !contract.IsRunning() || string(r.Process.AppliedConfig()) != "{}" {
		t.Fatal("invalid candidate displaced running core")
	}
	if err := os.WriteFile(path, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := contract.Restart(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := contract.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if contract.IsRunning() {
		t.Fatal("stopped core is still running")
	}
}
