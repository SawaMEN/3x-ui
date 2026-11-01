package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
