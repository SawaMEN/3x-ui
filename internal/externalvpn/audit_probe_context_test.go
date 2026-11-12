//go:build !windows

package externalvpn

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

func TestProbeCanceledWhileWaitingForOutboundLock(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	outboundMu.Lock()
	locked := true
	defer func() {
		if locked {
			outboundMu.Unlock()
		}
	}()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, cleanup, err := EnsureProbeOutboundContext(ctx, openFluxPlan("queued", "a"))
		if cleanup != nil {
			cleanup()
		}
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		outboundMu.Unlock()
		locked = false
		<-done
		t.Fatal("canceled probe waited for unrelated outbound setup")
	}
}

func TestProbeCancellationStopsOpenFluxAndRemovesFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	marker := filepath.Join(dir, "started")
	t.Setenv("XUI_AUDIT_PROBE_PID", marker)
	if err := os.WriteFile(filepath.Join(dir, "openflux"), []byte("#!/bin/sh\necho $$ > \"$XUI_AUDIT_PROBE_PID\"\nexec sleep 60\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, cleanup, err := EnsureProbeOutboundContext(ctx, openFluxPlan("owned", "a"))
		if cleanup != nil {
			cleanup()
		}
		done <- err
	}()
	var pid int
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		data, _ := os.ReadFile(marker)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		if pid > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("owned probe child did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("probe error=%v, want cancellation", err)
		}
	case <-time.After(time.Second):
		_ = syscall.Kill(pid, syscall.SIGKILL)
		<-done
		t.Fatal("probe did not stop on cancel")
	}
	if err := syscall.Kill(pid, 0); !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("probe child survived: %v", err)
	}
	files, err := filepath.Glob(filepath.Join(directory(), "outbound-*"))
	if err != nil || len(files) != 0 {
		t.Fatalf("private probe files remained: %v %v", files, err)
	}
}

func TestProbeCancellationInterruptsDockerInspection(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	marker := filepath.Join(dir, "inspect.pid")
	t.Setenv("XUI_AUDIT_INSPECT_PID", marker)
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte("#!/bin/sh\nif [ \"$1\" = image ]; then echo $$ > \"$XUI_AUDIT_INSPECT_PID\"; exec sleep 60; fi\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sudoku-tunnel"), []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sudoku.version"), []byte("fixture\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{"tag": "docker-probe", "protocol": "sudoku", "settings": map[string]any{"link": sudokuTestLink(t, nil)}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, cleanup, err := EnsureProbeOutboundContext(ctx, raw)
		if cleanup != nil {
			cleanup()
		}
		done <- err
	}()
	var pid int
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); {
		data, _ := os.ReadFile(marker)
		pid, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		if pid > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if pid == 0 {
		cancel()
		select {
		case err := <-done:
			t.Fatalf("inspection not reached: %v", err)
		case <-time.After(time.Second):
			t.Fatal("inspection not reached")
		}
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		_ = syscall.Kill(pid, syscall.SIGKILL)
		<-done
		t.Fatal("Docker inspection ignored probe cancellation")
	}
	files, _ := filepath.Glob(filepath.Join(directory(), "outbound-*"))
	if len(files) != 0 {
		t.Fatalf("canceled inspection retained private files: %v", files)
	}
}

func TestProbeContextDoesNotStopProductionOutbound(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	if err := os.WriteFile(filepath.Join(dir, "openflux"), []byte("#!/bin/sh\nexec sleep 60\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	raw := openFluxPlan("shared-tag", "a")
	if _, err := EnsureOutbound(raw); err != nil {
		t.Fatal(err)
	}
	defer KeepOutbounds(nil)
	outboundMu.Lock()
	production := outboundProcesses["shared-tag"]
	outboundMu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bridge, cleanup, err := EnsureProbeOutboundContext(ctx, raw)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if bridge["tag"] != "shared-tag" {
		t.Fatal("probe changed caller's tag")
	}
	outboundMu.Lock()
	var probe *managedOutbound
	for _, proc := range outboundProcesses {
		if proc.probe {
			probe = proc
		}
	}
	outboundMu.Unlock()
	if probe == nil || probe == production {
		t.Fatal("probe did not get an isolated child")
	}
	cancel()
	select {
	case <-probe.done:
	case <-time.After(time.Second):
		t.Fatal("probe child outlived its context")
	}
	select {
	case <-production.done:
		t.Fatal("probe canceled the production tunnel")
	default:
	}
	cleanup()
	cleanup()
	if !OutboundsApplied([]map[string]any{raw}) {
		t.Fatal("probe cleanup disturbed production identity")
	}
}

func TestProbeReadinessWaitsHonorCancellation(t *testing.T) {
	for _, protocol := range []string{"openflux", "vetroff", "sudoku", "fptn"} {
		t.Run(protocol, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				proc := &managedOutbound{raw: map[string]any{"protocol": protocol}, done: make(chan struct{}), cmd: &exec.Cmd{}}
				done := make(chan error, 1)
				go func() { done <- waitOutboundStartupContext(ctx, "synthetic", proc) }()
				synctest.Wait()
				cancel()
				if err := <-done; !errors.Is(err, context.Canceled) {
					t.Fatalf("%s wait error=%v", protocol, err)
				}
			})
		})
	}
}
