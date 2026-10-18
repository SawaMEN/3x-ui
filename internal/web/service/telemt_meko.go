package service

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pelletier/go-toml/v2"
)

const (
	telemtMekoEnvPath         = "/etc/x-ui/telemt-meko-fix.env"
	telemtMekoScriptPath      = "/usr/local/x-ui/telemt-meko-fix.sh"
	telemtMekoUnitPath        = "/etc/systemd/system/telemt-meko-fix.service"
	telemtMekoBundledUnitPath = "/usr/local/x-ui/telemt-meko-fix.service"
)

type TelemtMekoConfig struct {
	Enabled       bool   `json:"enabled"`
	Applied       bool   `json:"applied"`
	Healthy       bool   `json:"healthy"`
	Backend       string `json:"backend"`
	StatusError   string `json:"statusError,omitempty"`
	RatePerMinute int    `json:"ratePerMinute"`
	Burst         int    `json:"burst"`
	Ports         []int  `json:"ports"`
	AppliedPorts  []int  `json:"appliedPorts"`
	VerifiedPorts []int  `json:"verifiedPorts"`
	Fingerprint   string `json:"fingerprint"`
	Mark          string `json:"mark"`
}

func defaultTelemtMekoConfig() TelemtMekoConfig {
	return TelemtMekoConfig{
		Enabled:       runtime.GOOS == "linux",
		RatePerMinute: 54,
		Burst:         1,
		Fingerprint:   "MEKO V3/u32",
		Mark:          "0x400",
	}
}

func parseMekoBool(value string) bool {
	value = strings.ToLower(strings.TrimSpace(strings.Trim(value, "\"'")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func parseMekoRate(value string) int {
	value = strings.TrimSpace(strings.Trim(value, "\"'"))
	value = strings.TrimSuffix(value, "/minute")
	value = strings.TrimSuffix(value, "/min")
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return 0
	}
	return n
}

func parseMekoPortList(value string) []int {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	seen := make(map[int]struct{})
	for _, part := range strings.Split(value, ",") {
		p, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && p >= 1 && p <= 65535 {
			seen[p] = struct{}{}
		}
	}
	ports := make([]int, 0, len(seen))
	for p := range seen {
		ports = append(ports, p)
	}
	sort.Ints(ports)
	return ports
}

func readTelemtMekoEnv(cfg *TelemtMekoConfig) {
	data, err := os.ReadFile(telemtMekoEnvPath)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "TELEMT_MEKO_ENABLED":
			cfg.Enabled = parseMekoBool(value)
		case "TELEMT_MEKO_RATE":
			if n := parseMekoRate(value); n > 0 {
				cfg.RatePerMinute = n
			}
		case "TELEMT_MEKO_BURST":
			if n, err := strconv.Atoi(strings.TrimSpace(strings.Trim(value, "\"'"))); err == nil && n > 0 {
				cfg.Burst = n
			}
		}
	}
}

func telemtPortFromConfig(path string) int {
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(data), "# x-ui-meko-fix=1\n") {
		// Old sidecars, unselected ports and standalone WEB Proxy must never
		// acquire MEKO rules just because they share the Telemt executable.
		return 0
	}
	var raw struct {
		Server struct {
			Port int `toml:"port"`
		} `toml:"server"`
	}
	if err := toml.Unmarshal(data, &raw); err != nil {
		return 0
	}
	if raw.Server.Port < 1 || raw.Server.Port > 65535 {
		return 0
	}
	return raw.Server.Port
}

func discoverTelemtMekoPorts() []int {
	seen := make(map[int]struct{})
	pattern := filepath.Join(filepath.Dir(telemtBinaryPath), "mtproto", "telemt-*.toml")
	if files, err := filepath.Glob(pattern); err == nil {
		for _, path := range files {
			if port := telemtPortFromConfig(path); port > 0 {
				seen[port] = struct{}{}
			}
		}
	}
	ports := make([]int, 0, len(seen))
	for port := range seen {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
}

func mekoEnv(cfg TelemtMekoConfig) []string {
	enabled := "0"
	if cfg.Enabled {
		enabled = "1"
	}
	return []string{
		"TELEMT_MEKO_ENABLED=" + enabled,
		fmt.Sprintf("TELEMT_MEKO_RATE=%d/minute", cfg.RatePerMinute),
		fmt.Sprintf("TELEMT_MEKO_BURST=%d", cfg.Burst),
	}
}

func runTelemtMekoScript(action string, cfg TelemtMekoConfig) ([]byte, error) {
	if _, err := os.Stat(telemtMekoScriptPath); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, telemtMekoScriptPath, action)
	cmd.Env = append(os.Environ(), mekoEnv(cfg)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("telemt meko: %s failed: %w: %s", action, err, strings.TrimSpace(string(out)))
	}
	return out, nil
}

func readTelemtMekoRuntime(cfg *TelemtMekoConfig) error {
	out, err := runTelemtMekoScript("status", *cfg)
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(out), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch key {
		case "installed":
			cfg.Applied = value == "true"
		case "backend":
			cfg.Backend = value
		case "ports":
			cfg.Ports = parseMekoPortList(value)
		case "applied_ports":
			cfg.AppliedPorts = parseMekoPortList(value)
		case "verified_ports":
			cfg.VerifiedPorts = parseMekoPortList(value)
		}
	}
	return nil
}

func (TelemtService) GetMekoConfig() TelemtMekoConfig {
	cfg := defaultTelemtMekoConfig()
	readTelemtMekoEnv(&cfg)
	cfg.Ports = discoverTelemtMekoPorts()
	// Per-inbound selection owns whether MEKO is enabled. A previous global
	// toggle must not unexpectedly disable an opted-in ordinary MTProto port.
	cfg.Enabled = len(cfg.Ports) > 0
	if err := readTelemtMekoRuntime(&cfg); err != nil {
		cfg.StatusError = err.Error()
	}
	mekoApplyError.RLock()
	if cfg.StatusError == "" {
		cfg.StatusError = mekoApplyError.message
	}
	mekoApplyError.RUnlock()
	cfg.Healthy = cfg.StatusError == "" && (cfg.Enabled && cfg.Applied && sameMekoPorts(cfg.Ports, cfg.AppliedPorts) && sameMekoPorts(cfg.Ports, cfg.VerifiedPorts) || !cfg.Enabled && !cfg.Applied && len(cfg.AppliedPorts) == 0)
	if cfg.Ports == nil {
		cfg.Ports = []int{}
	}
	if cfg.AppliedPorts == nil {
		cfg.AppliedPorts = []int{}
	}
	if cfg.VerifiedPorts == nil {
		cfg.VerifiedPorts = []int{}
	}
	return cfg
}

func validateTelemtMekoConfig(cfg TelemtMekoConfig) error {
	if cfg.RatePerMinute < 1 || cfg.RatePerMinute > 60000 {
		return errors.New("telemt meko: rate must be between 1 and 60000 SYN/minute per IP")
	}
	if cfg.Burst < 1 || cfg.Burst > 1000 {
		return errors.New("telemt meko: burst must be between 1 and 1000")
	}
	return nil
}

func writeTelemtMekoEnv(cfg TelemtMekoConfig) error {
	if err := os.MkdirAll(filepath.Dir(telemtMekoEnvPath), 0o700); err != nil {
		return err
	}
	enabled := 0
	if cfg.Enabled {
		enabled = 1
	}
	data := fmt.Sprintf("TELEMT_MEKO_ENABLED=%d\nTELEMT_MEKO_RATE=%d/minute\nTELEMT_MEKO_BURST=%d\n", enabled, cfg.RatePerMinute, cfg.Burst)
	tmp, err := os.CreateTemp(filepath.Dir(telemtMekoEnvPath), ".telemt-meko-*.env")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.WriteString(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, telemtMekoEnvPath)
}

func ensureTelemtMekoAssets() error {
	if info, err := os.Stat(telemtMekoScriptPath); err != nil {
		return fmt.Errorf("telemt meko: fix script is missing; update/reinstall the panel package: %w", err)
	} else if !info.Mode().IsRegular() {
		return errors.New("telemt meko: fix script is not a regular file")
	} else if info.Mode().Perm()&0o111 == 0 {
		if err := os.Chmod(telemtMekoScriptPath, 0o755); err != nil {
			return err
		}
	}

	bundled, err := os.ReadFile(telemtMekoBundledUnitPath)
	if err != nil {
		return fmt.Errorf("telemt meko: service unit is missing: %w", err)
	}
	installed, readErr := os.ReadFile(telemtMekoUnitPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("telemt meko: read installed service unit: %w", readErr)
	}
	if errors.Is(readErr, os.ErrNotExist) || string(installed) != string(bundled) {
		if err := os.WriteFile(telemtMekoUnitPath, bundled, 0o644); err != nil {
			return fmt.Errorf("telemt meko: install service unit: %w", err)
		}
	}
	return systemctl("daemon-reload")
}

func (TelemtService) SaveMekoConfig(cfg TelemtMekoConfig) error {
	// MEKO activation is owned by selected ordinary MTProto inbounds, never
	// by the deprecated global Telemt switch or an arbitrary API payload.
	// Legacy callers may still change default rate/burst, but cannot disable
	// protection for an opted-in port or enable it on unrelated listeners.
	cfg.Enabled = len(discoverTelemtMekoPorts()) > 0
	if err := validateTelemtMekoConfig(cfg); err != nil {
		return err
	}
	if err := writeTelemtMekoEnv(cfg); err != nil {
		return fmt.Errorf("telemt meko: save settings: %w", err)
	}
	if cfg.Enabled {
		if err := ensureTelemtMekoAssets(); err != nil {
			return err
		}
		if err := systemctl("enable", telemtMekoServiceName); err != nil {
			return fmt.Errorf("telemt meko: enable failed: %w", err)
		}
		if err := systemctl("restart", telemtMekoServiceName); err != nil {
			return fmt.Errorf("telemt meko: apply failed: %w", err)
		}
		return nil
	}

	var errs []error
	if _, err := os.Stat(telemtMekoUnitPath); err == nil {
		if err := systemctl("disable", "--now", telemtMekoServiceName); err != nil {
			errs = append(errs, fmt.Errorf("telemt meko: disable failed: %w", err))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("telemt meko: inspect service unit: %w", err))
	}
	if _, err := os.Stat(telemtMekoScriptPath); err == nil {
		if _, err := runTelemtMekoScript("remove", cfg); err != nil {
			errs = append(errs, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		errs = append(errs, fmt.Errorf("telemt meko: inspect fix script: %w", err))
	}
	return errors.Join(errs...)
}

func sameMekoPorts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The last application error belongs to the MEKO runtime, not to a single
// request. Expose it alongside the actual kernel state instead of silently
// logging a failed firewall change after the user saved an inbound.
var mekoApplyError struct {
	sync.RWMutex
	message string
}

var mekoApplyState struct {
	sync.Mutex
	fingerprint string
}

// The applied port set alone cannot tell us if the per-inbound rate or burst
// changed. Hash only the MEKO metadata of selected configs, not client secrets
// or volatile Telemt process settings.
func desiredMekoPolicyFingerprint(cfg TelemtMekoConfig) string {
	var b strings.Builder
	fmt.Fprintf(&b, "rate=%d,burst=%d;", cfg.RatePerMinute, cfg.Burst)
	pattern := filepath.Join(filepath.Dir(telemtBinaryPath), "mtproto", "telemt-*.toml")
	paths, _ := filepath.Glob(pattern)
	sort.Strings(paths)
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil || !strings.HasPrefix(string(data), "# x-ui-meko-fix=1\n") {
			continue
		}
		fmt.Fprintf(&b, "%d;", telemtPortFromConfig(path))
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "# x-ui-meko-rate=") || strings.HasPrefix(line, "# x-ui-meko-burst=") || strings.HasPrefix(line, "# x-ui-meko-ios-rate=") || strings.HasPrefix(line, "# x-ui-meko-ios-burst=") {
				b.WriteString(line)
				b.WriteByte(';')
			}
			if strings.HasPrefix(line, "[general]") {
				break
			}
		}
	}
	return fmt.Sprintf("%x", sha256.Sum256([]byte(b.String())))
}

// RefreshMekoFix reconciles the MEKO firewall rules with the Telemt processes
// currently generated from MTProto inbounds. It is intentionally a no-op when
// the configured and applied port sets already match, so the hashlimit buckets
// are not reset by every MTProto traffic polling cycle.
func (TelemtService) RefreshMekoFix() (applyErr error) {
	defer func() {
		mekoApplyError.Lock()
		if applyErr != nil {
			mekoApplyError.message = applyErr.Error()
		} else {
			mekoApplyError.message = ""
		}
		mekoApplyError.Unlock()
	}()
	mekoApplyState.Lock()
	defer mekoApplyState.Unlock()
	cfg := TelemtService{}.GetMekoConfig()
	fingerprint := desiredMekoPolicyFingerprint(cfg)
	if !cfg.Enabled {
		mekoApplyState.fingerprint = ""
		// A disabled/removed final inbound should also disable the owned
		// oneshot service. Otherwise systemd can re-create obsolete rules on
		// the next panel start even though no MTProto port opted in.
		if systemctl("is-enabled", "--quiet", telemtMekoServiceName) == nil {
			if err := systemctl("disable", "--now", telemtMekoServiceName); err != nil {
				return fmt.Errorf("telemt meko: disable unused unit: %w", err)
			}
		}
		// Even an incomplete or damaged owned ruleset must be removed once
		// the last opted-in MTProto inbound disappears. Otherwise stale
		// firewall rules can survive forever with Applied=false.
		if cfg.Applied || cfg.Backend != "" && cfg.Backend != "none" || len(cfg.AppliedPorts) > 0 {
			_, err := runTelemtMekoScript("remove", cfg)
			return err
		}
		return nil
	}
	if !cfg.Applied && len(cfg.Ports) == 0 {
		return nil
	}
	if cfg.Healthy && sameMekoPorts(cfg.Ports, cfg.VerifiedPorts) &&
		mekoApplyState.fingerprint == fingerprint {
		return nil
	}
	if err := ensureTelemtMekoAssets(); err != nil {
		return err
	}
	if _, err := os.Stat(telemtMekoEnvPath); errors.Is(err, os.ErrNotExist) {
		if err := writeTelemtMekoEnv(cfg); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("telemt meko: inspect settings: %w", err)
	}
	// An existing env file does not imply the systemd unit was enabled.
	// Repair unit enablement automatically so the selected ports survive
	// reboot/panel restart even on upgraded installations.
	if systemctl("is-enabled", "--quiet", telemtMekoServiceName) != nil {
		if err := systemctl("enable", "--now", telemtMekoServiceName); err != nil {
			return fmt.Errorf("telemt meko: enable service: %w", err)
		}
	}
	_, err := runTelemtMekoScript("apply", cfg)
	if err != nil {
		return err
	}
	// Re-read the kernel rule set: a zero exit code is not proof that both
	// families and every selected listener were actually installed.
	applied := TelemtService{}.GetMekoConfig()
	// A previous failed apply may still be shown in mekoApplyError until this
	// invocation returns. Verify fresh kernel state, not cached error history.
	if !applied.Applied || !sameMekoPorts(applied.Ports, applied.AppliedPorts) || !sameMekoPorts(applied.Ports, applied.VerifiedPorts) {
		return fmt.Errorf("telemt meko: firewall verification failed: desired=%v applied=%v backend=%s error=%s", applied.Ports, applied.AppliedPorts, applied.Backend, applied.StatusError)
	}
	mekoApplyState.fingerprint = fingerprint
	return nil
}
