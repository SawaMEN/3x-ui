package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
)

const firewallPingEnabledKey = "firewallPingEnabled"

var firewallPingSysctlPaths = []string{
	"/proc/sys/net/ipv4/icmp_echo_ignore_all",
	"/proc/sys/net/ipv6/icmp/echo_ignore_all",
}

func readFirewallPingEnabled() (bool, error) {
	found := false
	for _, path := range firewallPingSysctlPaths {
		raw, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return false, fmt.Errorf("read ping state %s: %w", path, err)
		}
		found = true
		rawValue := strings.TrimSpace(string(raw))
		if rawValue != "0" && rawValue != "1" {
			return false, fmt.Errorf("invalid ping state in %s", path)
		}
		if rawValue == "1" {
			return false, nil
		}
	}
	if !found {
		return true, nil
	}
	return true, nil
}

func writeFirewallPingEnabled(enabled bool) error {
	value := "1\n"
	if enabled {
		value = "0\n"
	}
	found := false
	var errs []error
	for _, path := range firewallPingSysctlPaths {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			errs = append(errs, fmt.Errorf("inspect ping state %s: %w", path, err))
			continue
		}
		found = true
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			errs = append(errs, fmt.Errorf("open ping state %s: %w", path, err))
			continue
		}
		_, writeErr := file.WriteString(value)
		closeErr := file.Close()
		if writeErr != nil {
			errs = append(errs, fmt.Errorf("write ping state %s: %w", path, writeErr))
		} else if closeErr != nil {
			errs = append(errs, fmt.Errorf("close ping state %s: %w", path, closeErr))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	if !found {
		return errors.New("ICMP echo sysctl is unavailable")
	}
	return nil
}

func firewallPingPreference() (enabled, configured bool, err error) {
	setting, err := (&SettingService{}).getSetting(firewallPingEnabledKey)
	if database.IsNotFound(err) {
		current, readErr := readFirewallPingEnabled()
		return current, false, readErr
	}
	if err != nil {
		return false, false, err
	}
	raw := strings.TrimSpace(setting.Value)
	if raw == "" {
		return true, true, nil
	}
	enabled, err = strconv.ParseBool(raw)
	return enabled, true, err
}

func (s *FirewallService) reconcileManagedPingStateLocked(backendEnabled bool) error {
	preferred, configured, err := firewallPingPreference()
	if err != nil || !configured {
		return err
	}
	if !backendEnabled {
		return writeFirewallPingEnabled(true)
	}
	return writeFirewallPingEnabled(preferred)
}

func (s *FirewallService) ReconcileManagedPingState(ctx context.Context, safetyPort int) (FirewallManagedStatus, error) {
	firewallMu.Lock()
	defer firewallMu.Unlock()

	backend, err := detectManagedFirewallBackend(ctx)
	if err != nil {
		return FirewallManagedStatus{}, err
	}
	on, err := backend.enabled(ctx)
	if err != nil {
		return FirewallManagedStatus{}, err
	}
	if err := s.reconcileManagedPingStateLocked(on); err != nil {
		return FirewallManagedStatus{}, err
	}
	return s.managedStatusSafeLocked(ctx, safetyPort)
}

func (s *FirewallService) SetManagedPingEnabledSafe(ctx context.Context, enabled bool, safetyPort int) (FirewallManagedStatus, error) {
	firewallMu.Lock()
	defer firewallMu.Unlock()

	backend, err := detectManagedFirewallBackend(ctx)
	if err != nil {
		return FirewallManagedStatus{}, err
	}
	on, err := backend.enabled(ctx)
	if err != nil {
		return FirewallManagedStatus{}, err
	}

	previousEffective, readErr := readFirewallPingEnabled()
	if readErr != nil {
		return FirewallManagedStatus{}, readErr
	}
	if on {
		if err := writeFirewallPingEnabled(enabled); err != nil {
			return FirewallManagedStatus{}, err
		}
	}
	if err := (&SettingService{}).setBool(firewallPingEnabledKey, enabled); err != nil {
		if on {
			_ = writeFirewallPingEnabled(previousEffective)
		}
		return FirewallManagedStatus{}, err
	}
	return s.managedStatusSafeLocked(ctx, safetyPort)
}

func (status FirewallManagedStatus) MarshalJSON() ([]byte, error) {
	type statusAlias FirewallManagedStatus
	pingEnabled, _, err := firewallPingPreference()
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		statusAlias
		PingEnabled bool `json:"pingEnabled"`
	}{statusAlias: statusAlias(status), PingEnabled: pingEnabled})
}
