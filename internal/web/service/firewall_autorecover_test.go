package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resetNetfilterRecoveryForTest(t *testing.T) {
	t.Helper()
	netfilterRecovery.Lock()
	old := netfilterRecovery.lastAttempt
	netfilterRecovery.lastAttempt = time.Time{}
	netfilterRecovery.Unlock()
	t.Cleanup(func() {
		netfilterRecovery.Lock()
		netfilterRecovery.lastAttempt = old
		netfilterRecovery.Unlock()
	})
}

func TestFirewallAutomaticallyLoadsNftModulesAndRetries(t *testing.T) {
	resetNetfilterRecoveryForTest(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	ready := filepath.Join(dir, "nft-ready")
	fakeFirewallBinary(t, dir, "nft", fmt.Sprintf(`if [ "$1" = "-j" ] && [ -f %q ]; then
  echo '{"nftables":[]}'
  exit 0
fi
echo 'netlink: Error: cache initialization failed: Invalid argument' >&2
exit 1`, ready))
	fakeFirewallBinary(t, dir, "modprobe", fmt.Sprintf(`if [ "$2" = "nf_tables" ]; then : > %q; fi
exit 0`, ready))

	backend, err := detectManagedFirewallBackend(context.Background())
	if err != nil {
		t.Fatalf("expected automatic module recovery, got %v", err)
	}
	if backend.name != "nftables" {
		t.Fatalf("expected recovered nftables backend, got %+v", backend)
	}
	if _, err := os.Stat(ready); err != nil {
		t.Fatalf("nf_tables was not loaded: %v", err)
	}
}

func TestFirewallNeverLoadsModulesToBypassPermissions(t *testing.T) {
	resetNetfilterRecoveryForTest(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	invoked := filepath.Join(dir, "modprobe-invoked")
	fakeFirewallBinary(t, dir, "nft", "echo 'netlink: Error: cache initialization failed: Permission denied' >&2; exit 1")
	fakeFirewallBinary(t, dir, "modprobe", fmt.Sprintf(": > %q; exit 0", invoked))

	_, err := detectManagedFirewallBackend(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("expected permissions failure, got %v", err)
	}
	if _, err := os.Stat(invoked); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("module loading must not be attempted after a permissions error: %v", err)
	}
}

func TestFirewallModuleRecoveryIsRateLimited(t *testing.T) {
	resetNetfilterRecoveryForTest(t)
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	count := filepath.Join(dir, "attempts")
	fakeFirewallBinary(t, dir, "modprobe", fmt.Sprintf("echo attempt >> %q; exit 1", count))
	failures := []error{errors.New("netlink: Error: cache initialization failed: Invalid argument")}
	for range 2 {
		if tryRecoverManagedFirewallKernel(context.Background(), failures) {
			t.Fatal("failed modprobe must not be reported as successful")
		}
	}
	data, err := os.ReadFile(count)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(string(data), "attempt"); got != 3 {
		t.Fatalf("expected one three-module recovery attempt, got %d commands", got)
	}
}

func TestFirewallRecoveryDiagnosisPermissionDenied(t *testing.T) {
	code, kernel := firewallRecoveryDiagnosis(errors.New("iptables: Permission denied"))
	if code != "permission-denied" || kernel != "" {
		t.Fatalf("unexpected diagnostic: code=%q, kernel=%q", code, kernel)
	}
}
