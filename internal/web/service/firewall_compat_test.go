package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFirewallNftCompatibilityClassification(t *testing.T) {
	for _, tc := range []struct {
		message string
		want    bool
	}{
		{"iptables v1.8.13 (nf_tables): Could not fetch rule set generation id: Invalid argument", true},
		{"netlink: Error: cache initialization failed: Operation not supported", true},
		{"inspect nftables ruleset: netlink: Error: cache initialization failed: Invalid argument", true},
		{"netlink: Error: cache initialization failed: Permission denied", false},
		{"nft: syntax error: Invalid argument", false},
		{"Could not fetch rule set generation id: Permission denied (you must be root)", false},
		{"command timed out", false},
	} {
		if got := isNftablesCompatibilityError(errors.New(tc.message)); got != tc.want {
			t.Errorf("%s: got %v", tc.message, got)
		}
	}
}

func fakeFirewallBinary(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestFirewallFallsBackToLegacyOnIncompatibleNft(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	failure := "echo 'netlink: Error: cache initialization failed: Invalid argument' >&2; exit 1"
	fakeFirewallBinary(t, dir, "nft", failure)
	fakeFirewallBinary(t, dir, "iptables", failure)
	fakeFirewallBinary(t, dir, "iptables-legacy", "if [ \"$1\" = '-S' ]; then exit 0; fi; exit 1")
	backend, err := detectManagedFirewallBackend(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if backend.name != "iptables" || filepath.Base(backend.binary) != "iptables-legacy" {
		t.Fatalf("unexpected backend: %+v", backend)
	}
}

func TestFirewallDoesNotBypassNftPermissionFailure(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	fakeFirewallBinary(t, dir, "nft", "echo 'Permission denied' >&2; exit 1")
	fakeFirewallBinary(t, dir, "iptables-legacy", "if [ \"$1\" = '-S' ]; then exit 0; fi; exit 1")
	_, err := detectManagedFirewallBackend(context.Background())
	if err == nil || !strings.Contains(err.Error(), "Permission denied") {
		t.Fatalf("expected permission error, got %v", err)
	}
}

func TestFirewallLegacyIPv6UsesMatchingImplementation(t *testing.T) {
	for _, tc := range []struct{ binary, want string }{
		{"/usr/sbin/iptables-legacy", "ip6tables-legacy"},
		{"/usr/sbin/iptables-nft", "ip6tables-nft"},
		{"/usr/sbin/iptables", "ip6tables"},
	} {
		if got := ip6tablesBinary(tc.binary); got != tc.want {
			t.Errorf("%s: got %s", tc.binary, got)
		}
	}
}

func TestFirewallLegacyRequiresMatchingIPv6BeforeApplyingRules(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	// Running IPv4 would fail with this sentinel. Missing IPv6 must be caught
	// before any IPv4 rules are changed.
	binary := filepath.Join(dir, "iptables-legacy")
	fakeFirewallBinary(t, dir, "iptables-legacy", "echo 'IPv4 was invoked' >&2; exit 1")
	err := applyManagedIPTablesSafe(context.Background(), binary, nil)
	if err == nil || !strings.Contains(err.Error(), "ip6tables-legacy is required") {
		t.Fatalf("expected IPv6 preflight failure, got %v", err)
	}
}
