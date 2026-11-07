package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Module loading is the only automatic host-level repair. Package upgrades,
// changes to system firewall alternatives, and reboots require operator control
// so a status request cannot disrupt SSH or an existing host firewall.
const netfilterRecoveryCooldown = 5 * time.Minute

var netfilterRecovery = struct {
	sync.Mutex
	lastAttempt time.Time
}{}

func tryRecoverManagedFirewallKernel(ctx context.Context, failures []error) bool {
	if ctx.Err() != nil || len(failures) == 0 {
		return false
	}

	nft, legacy := false, false
	for _, failure := range failures {
		if failure == nil {
			continue
		}
		// Never hide permission, syntax, or unexpected failures by trying a
		// different backend or modifying the host.
		if !isFirewallKernelCompatibilityError(failure) {
			return false
		}
		nft = nft || isNftablesCompatibilityError(failure)
		legacy = legacy || isLegacyIPTablesKernelError(failure)
	}
	if !nft && !legacy {
		return false
	}

	modprobe, err := exec.LookPath("modprobe")
	if err != nil {
		return false
	}

	netfilterRecovery.Lock()
	now := time.Now()
	if now.Sub(netfilterRecovery.lastAttempt) < netfilterRecoveryCooldown {
		netfilterRecovery.Unlock()
		return false
	}
	netfilterRecovery.lastAttempt = now
	netfilterRecovery.Unlock()

	var modules []string
	if nft {
		modules = append(modules, "nfnetlink", "nf_tables", "nft_chain_filter")
	}
	if legacy {
		modules = append(modules, "ip_tables", "iptable_filter", "ip6_tables", "ip6table_filter")
	}

	loaded := false
	for _, module := range modules {
		if ctx.Err() != nil {
			break
		}
		if _, err := runFirewallCommand(ctx, modprobe, "-q", module); err == nil {
			loaded = true
		}
	}
	// A successful modprobe does not prove a usable firewall. The caller
	// retries real ruleset inspection once before selecting any backend.
	return loaded
}

func firewallRecoveryDiagnosis(err error) (code, kernel string) {
	if err == nil {
		return "", ""
	}
	message := strings.ToLower(err.Error())
	if strings.Contains(message, "permission denied") ||
		strings.Contains(message, "operation not permitted") {
		return "permission-denied", ""
	}
	if !isFirewallKernelCompatibilityError(err) {
		return "", ""
	}
	release, readErr := os.ReadFile("/proc/sys/kernel/osrelease")
	if readErr == nil {
		kernel = strings.TrimSpace(string(release))
	}
	if kernel != "" {
		_, moduleErr := os.Stat(filepath.Join("/lib/modules", kernel))
		if errors.Is(moduleErr, os.ErrNotExist) {
			return "kernel-modules-missing", kernel
		}
	}
	return "kernel-netfilter-unavailable", kernel
}
