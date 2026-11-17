package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// Only kernel/backend compatibility errors permit trying another backend.
// Permission failures must remain visible instead of being treated as inactive.
func isNftablesCompatibilityError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "could not fetch rule set generation id: invalid argument") ||
		strings.Contains(text, "cache initialization failed: invalid argument") ||
		strings.Contains(text, "operation not supported") ||
		strings.Contains(text, "protocol not supported") ||
		strings.Contains(text, "address family not supported")
}

func ufwConfiguredEnabled() bool {
	data, err := os.ReadFile("/etc/ufw/ufw.conf")
	if err != nil {
		return !os.IsNotExist(err)
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.TrimSpace(key) == "ENABLED" {
			return strings.Trim(strings.TrimSpace(value), "\"'") == "yes"
		}
	}
	return false
}

// Keep both address families on the same implementation during apply/remove.
func ip6tablesBinary(binary string) string {
	switch filepath.Base(binary) {
	case "iptables-legacy":
		return "ip6tables-legacy"
	case "iptables-nft":
		return "ip6tables-nft"
	default:
		return "ip6tables"
	}
}

// A legacy IPv4 fallback must never silently skip IPv6 or use nft IPv6 tools.
func managedIP6TablesPath(binary string) (string, error) {
	name := ip6tablesBinary(binary)
	path, err := exec.LookPath(name)
	if err != nil && name != "ip6tables" {
		return "", fmt.Errorf("%s is required alongside %s: %w", name, binary, err)
	}
	return path, nil
}

// A legacy binary can exist even when the running kernel has no ip_tables
// support. This is a backend compatibility failure, not a permission failure.
func isLegacyIPTablesKernelError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "module ip_tables not found") ||
		strings.Contains(message, "module ip6_tables not found") ||
		(strings.Contains(message, "table does not exist") &&
			(strings.Contains(message, "can't initialize iptables table") ||
				strings.Contains(message, "can't initialize ip6tables table")))
}

func isFirewallKernelCompatibilityError(err error) bool {
	return isNftablesCompatibilityError(err) || isLegacyIPTablesKernelError(err)
}

// Loading an already-installed module is non-destructive. Never install kernel
// packages, change system alternatives or reboot a remote host from a status
// request: doing so can sever SSH and leave its existing firewall unmanaged.
// A failed probe is remembered for this process to avoid repeated modprobe
// attempts on every UI refresh.
var legacyIPTablesModuleProbe sync.Once

func tryLoadLegacyIPTablesModules(ctx context.Context, backendError error) bool {
	if !isLegacyIPTablesKernelError(backendError) {
		return false
	}
	recovered := false
	legacyIPTablesModuleProbe.Do(func() {
		modprobe, err := exec.LookPath("modprobe")
		if err != nil {
			return
		}
		for _, name := range []string{"ip_tables", "iptable_filter", "ip6_tables", "ip6table_filter"} {
			if ctx.Err() != nil {
				return
			}
			if _, err := runFirewallCommand(ctx, modprobe, "-q", name); err == nil {
				recovered = true
			}
		}
	})
	return recovered
}
