package sys

import (
	"fmt"
	"math"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
)

const (
	memLimitHeadroomPercent = 90
	defaultGCPercent        = 75
	lowResourceGCPercent    = 50
	defaultReleaseMinutes   = 10
	lowResourceReleaseMin   = 5
	lowResourceSoftLimitMiB = 256
	bytesPerMiB             = int64(1 << 20)
)

// ApplyMemoryTuning configures the Go runtime for a lower, steadier footprint.
// Explicit GOMEMLIMIT/XUI_MEMORY_LIMIT always wins. On small VDS hosts, a
// conservative soft heap budget is applied automatically so short-lived
// allocation bursts do not permanently inflate RSS.
func ApplyMemoryTuning() []string {
	lines := []string{applyGCPercent()}
	if limit, source := applyMemoryLimit(); limit > 0 {
		lines = append(lines, fmt.Sprintf("Go memory soft limit set to %d MiB (%s)", limit>>20, source))
	} else {
		lines = append(lines, "Go memory soft limit not enforced: "+source)
	}
	return lines
}

// applyGCPercent lowers GOGC so the heap high-water mark, and thus RSS, stays
// smaller. An explicit GOGC env (including GOGC=off) is left to the runtime.
func applyGCPercent() string {
	if _, ok := os.LookupEnv("GOGC"); ok {
		return "GC percent: GOGC env (handled by the Go runtime)"
	}

	pct := defaultGCPercent
	if IsLowResourceMode() {
		pct = lowResourceGCPercent
	}
	if v := strings.TrimSpace(os.Getenv("XUI_GOGC")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			pct = n
		}
	}

	if pct <= 0 {
		return "GC percent left at Go default"
	}
	debug.SetGCPercent(pct)
	return fmt.Sprintf("GC percent set to %d", pct)
}

// applyMemoryLimit respects explicit budgets first, then a real cgroup limit.
// On small VDS hosts the automatic budget is capped at 256 MiB to keep heap
// growth from dominating process RSS.
func applyMemoryLimit() (int64, string) {
	if strings.TrimSpace(os.Getenv("GOMEMLIMIT")) != "" {
		return 0, "GOMEMLIMIT env (handled by the Go runtime)"
	}

	if v := strings.TrimSpace(os.Getenv("XUI_MEMORY_LIMIT")); v != "" {
		if mb, err := strconv.ParseInt(v, 10, 64); err == nil && mb > 0 && mb <= math.MaxInt64/bytesPerMiB {
			limit := mb * bytesPerMiB
			debug.SetMemoryLimit(limit)
			return limit, "XUI_MEMORY_LIMIT=" + v + "MiB"
		}
	}

	if v, ok := cgroupMemoryLimit(); ok {
		limit := v / 100 * memLimitHeadroomPercent
		if IsLowResourceMode() && limit > lowResourceSoftLimitMiB*bytesPerMiB {
			limit = lowResourceSoftLimitMiB * bytesPerMiB
		}
		debug.SetMemoryLimit(limit)
		return limit, "cgroup limit"
	}

	if IsLowResourceMode() {
		limit := int64(lowResourceSoftLimitMiB) * bytesPerMiB
		debug.SetMemoryLimit(limit)
		return limit, "automatic low-resource profile"
	}

	return 0, "no explicit budget; soft limit left at Go default"
}

// MemoryReleaseIntervalMinutes reports how often freed heap memory is returned to
// the OS via debug.FreeOSMemory. XUI_MEMORY_RELEASE_INTERVAL overrides the
// default; an explicit 0 disables the periodic release.
func MemoryReleaseIntervalMinutes() int {
	v := strings.TrimSpace(os.Getenv("XUI_MEMORY_RELEASE_INTERVAL"))
	if v == "" {
		if IsLowResourceMode() {
			return lowResourceReleaseMin
		}
		return defaultReleaseMinutes
	}
	if n, err := strconv.Atoi(v); err == nil && n >= 0 {
		return n
	}
	return defaultReleaseMinutes
}

// cgroupMemoryLimit reads the container memory limit from cgroup v2 then v1.
// A "max" value or the v1 unlimited sentinel (~8 EiB) means no limit at this
// level, so it reports not-found and the caller falls back to the Go default. The
// files are absent off Linux, which also yields not-found.
func cgroupMemoryLimit() (int64, bool) {
	const unlimited = int64(1) << 62

	if b, err := os.ReadFile("/sys/fs/cgroup/memory.max"); err == nil {
		if s := strings.TrimSpace(string(b)); s != "" && s != "max" {
			if v, err := strconv.ParseInt(s, 10, 64); err == nil && v > 0 && v < unlimited {
				return v, true
			}
		}
	}

	if b, err := os.ReadFile("/sys/fs/cgroup/memory/memory.limit_in_bytes"); err == nil {
		if v, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil && v > 0 && v < unlimited {
			return v, true
		}
	}

	return 0, false
}
