//go:build linux

package service

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestStandaloneUpdatePreservesStoppedCoreOnDownloadFailure(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	svc := NativeCore(CoreTypeXray)
	if err := os.WriteFile(svc.BinaryPath(), []byte("original"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := updateStandaloneCore(context.Background(), CoreTypeXray, func(path string) (string, error) {
		if err := os.WriteFile(path, []byte("incomplete"), 0o700); err != nil {
			t.Fatal(err)
		}
		return "", errors.New("download failed")
	})
	if err == nil {
		t.Fatal("download failure was ignored")
	}
	raw, _ := os.ReadFile(svc.BinaryPath())
	if string(raw) != "original" {
		t.Fatal("failed update replaced original binary")
	}
	version, err := updateStandaloneCore(context.Background(), CoreTypeXray, func(path string) (string, error) { return "new", os.WriteFile(path, []byte("updated"), 0o700) })
	if err != nil || version != "new" {
		t.Fatal(version, err)
	}
	if svc.IsRunning() {
		t.Fatal("update started a stopped core")
	}
}

func TestStandaloneUpdateRestartsOldCoreAfterStartupFailure(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	previous := standaloneXrayProcess
	standaloneXrayProcess = standaloneProcess(CoreTypeXray, standaloneConfigPath(CoreTypeXray), true)
	t.Cleanup(func() { _ = standaloneXrayProcess.Stop(); standaloneXrayProcess = previous })
	svc := NativeCore(CoreTypeXray)
	old := "#!/bin/sh\nif [ \"$1\" = version ]; then echo 'Xray 26.10.21'; exit 0; fi\nif [ \"$2\" = -test ]; then exit 0; fi\nexec sleep 60\n"
	if err := os.WriteFile(svc.BinaryPath(), []byte(old), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(svc.ProcessConfigPath(), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := svc.process().Start(ctx); err != nil {
		t.Fatal(err)
	}
	broken := strings.Replace(old, "exec sleep 60", "echo startup-failed >&2; exit 1", 1)
	_, err := updateStandaloneCore(ctx, CoreTypeXray, func(path string) (string, error) { return "26.10.22", os.WriteFile(path, []byte(broken), 0o700) })
	if err == nil {
		t.Fatal("startup failure was ignored")
	}
	raw, _ := os.ReadFile(svc.BinaryPath())
	if string(raw) != old {
		t.Fatal("previous executable was not restored")
	}
	if !svc.IsRunning() {
		t.Fatal("previous core was not restarted")
	}
}
