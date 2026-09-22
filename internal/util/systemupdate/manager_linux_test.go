//go:build linux

package systemupdate

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseAptUpdates(t *testing.T) {
	got := parseAptUpdates(`Listing... Done
curl/noble-updates 8.5.0-1 amd64 [upgradable from: 8.4.0-1]
openssl/noble-updates 3.0.0 amd64 [upgradable from: 2.9.0]
`)
	if got["curl"] != "8.5.0-1" || got["openssl"] != "3.0.0" {
		t.Fatalf("parseAptUpdates() = %#v", got)
	}
}

func TestParseDnfUpdates(t *testing.T) {
	got := parseDnfUpdates(`kernel-core.x86_64 6.12.1-1.el9 baseos
curl.x86_64 8.5.0-1 appstream
Last metadata expiration check: 1:00:00 ago
`)
	if got["kernel-core"] != "6.12.1-1.el9" || got["curl"] != "8.5.0-1" {
		t.Fatalf("parseDnfUpdates() = %#v", got)
	}
}

func TestParseZypperUpdates(t *testing.T) {
	got := parseZypperUpdates(`v | repo | Name | Current Version | Available Version | Arch
--+------+------+
v | main | curl | 8.4.0 | 8.5.0 | x86_64
`)
	if got["curl"] != "8.5.0" {
		t.Fatalf("parseZypperUpdates() = %#v", got)
	}
}

func TestParsePacmanUpdates(t *testing.T) {
	got := parsePacmanUpdates(`curl 8.4.0-1 -> 8.5.0-1
linux 6.10.1-1 -> 6.10.2-1
`)
	if got["curl"] != "8.5.0-1" || got["linux"] != "6.10.2-1" {
		t.Fatalf("parsePacmanUpdates() = %#v", got)
	}
}

func TestParsePacmanQueryUpdates(t *testing.T) {
	got := parsePacmanQueryUpdates(`curl 8.4.0-1 -> 8.5.0-1
linux 6.10.1-1 -> 6.10.2-1
`)
	if got["curl"] != "8.5.0-1" || got["linux"] != "6.10.2-1" {
		t.Fatalf("parsePacmanQueryUpdates() = %#v", got)
	}
}

func TestParseApkUpdates(t *testing.T) {
	got := parseApkUpdates(`curl-8.4.0-r0 < 8.5.0-r0
linux-lts-6.6.1-r0 < 6.6.2-r0
`)
	if got["curl"] != "8.5.0-r0" || got["linux-lts"] != "6.6.2-r0" {
		t.Fatalf("parseApkUpdates() = %#v", got)
	}
}

func TestIsKernelPackage(t *testing.T) {
	for _, name := range []string{"linux-image-generic", "linux-lts", "kernel-core", "kernel-default"} {
		if !isKernelPackage(name) {
			t.Fatalf("isKernelPackage(%q) = false", name)
		}
	}
	for _, name := range []string{"curl", "openssl", "tzdata"} {
		if isKernelPackage(name) {
			t.Fatalf("isKernelPackage(%q) = true", name)
		}
	}
}

func TestStartAsyncCommandIgnoresCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	marker := filepath.Join(t.TempDir(), "started")
	err := startAsyncCommand(ctx, "sh", "-c", "printf started > \"$1\"", "sh", marker)
	if err != nil {
		t.Fatalf("startAsyncCommand() error = %v", err)
	}

	for i := 0; i < 50; i++ {
		if data, err := os.ReadFile(marker); err == nil && string(data) == "started" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("async command did not run after context cancellation")
}

func TestUpdateContextIgnoresCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	updateCtx, updateCancel := newUpdateContext(ctx)
	defer updateCancel()

	select {
	case <-updateCtx.Done():
		t.Fatalf("update context was canceled with the request context")
	default:
	}
}
