package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

const (
	firewallControlInitializedKey = "firewallControlInitialized"
	firewallSafetyPortKey         = "firewallSafetyPort"
	firewallSyncIntervalKey       = "firewallSyncInterval"
)

func ValidateFirewallSyncInterval(seconds int) error {
	if seconds < 5 || seconds > 3600 {
		return fmt.Errorf("firewall sync interval must be 5..3600 seconds")
	}
	return nil
}

func firewallSyncInterval() time.Duration {
	raw, err := firewallSetting(firewallSyncIntervalKey, "60")
	seconds, parseErr := strconv.Atoi(raw)
	if err != nil || parseErr != nil || ValidateFirewallSyncInterval(seconds) != nil {
		seconds = 60
	}
	return time.Duration(seconds) * time.Second
}

func (s *FirewallService) MarkControlInitialized() error {
	if firewallControlInitialized() {
		return nil
	}
	if err := (&SettingService{}).setBool(firewallControlInitializedKey, true); err != nil {
		return err
	}
	TriggerFirewallSync()
	return nil
}

// RememberSafetyPort persists the externally visible panel port discovered on
// an authenticated firewall-management request. Do not replace a previously
// learned external reverse-proxy port with the panel's internal listen port:
// doing so would let the next background reconcile close the public entrypoint.
func (s *FirewallService) RememberSafetyPort(port int) error {
	if port < 1 || port > 65535 {
		return nil
	}
	settings := &SettingService{}
	panelPort, _ := settings.GetPort()
	previous := rememberedFirewallSafetyPort()
	if previous == port {
		return nil
	}
	if previous > 0 && previous != panelPort && port == panelPort {
		return nil
	}
	return settings.setInt(firewallSafetyPortKey, port)
}

func rememberedFirewallSafetyPort() int {
	raw, err := firewallSetting(firewallSafetyPortKey, "0")
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || port < 1 || port > 65535 {
		return 0
	}
	return port
}

func firewallControlInitialized() bool {
	raw, err := firewallSetting(firewallControlInitializedKey, "false")
	if err != nil {
		return false
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false
	}
	enabled, err := strconv.ParseBool(raw)
	return err == nil && enabled
}

// SetAutoSyncPreference changes automatic inbound reconciliation. Turning it
// off deliberately freezes the rules currently present instead of immediately
// removing every 3x-ui-owned inbound rule; the operator may still use Sync now
// or manual rules while automatic reconciliation is disabled.
func (s *FirewallService) SetAutoSyncPreference(ctx context.Context, enabled bool, safetyPort int) (FirewallStatus, error) {
	firewallMu.Lock()
	defer firewallMu.Unlock()

	settings := &SettingService{}
	if !enabled {
		if err := settings.setBool(firewallAutoSyncKey, false); err != nil {
			return FirewallStatus{}, err
		}
		return s.status(ctx, safetyPort)
	}

	backend, err := detectFirewallBackend(ctx)
	if err != nil {
		return FirewallStatus{}, err
	}
	if err := settings.setBool(firewallAutoSyncKey, true); err != nil {
		return FirewallStatus{}, err
	}
	if err := s.sync(ctx, backend, safetyPort); err != nil {
		return FirewallStatus{}, err
	}
	return s.status(ctx, safetyPort)
}

// RunAutoSync keeps firewall rules aligned with enabled local inbounds even
// when changes arrive through imports, API calls, or node synchronization.
// Inbound API mutations trigger an immediate coalesced reconcile; a periodic
// configurable pass remains as drift repair for imports and future mutation
// paths and keeps the managed iptables jump behind newly added admin rules.
func (s *FirewallService) RunAutoSync(parent context.Context) {
	go ServeCertificateRenewalHookSocket(parent)
	go RunDomainCertificateRenewalScheduler(parent)
	syncNow := func(explicit bool) {
		if parent.Err() != nil {
			return
		}
		if !firewallControlInitialized() {
			return
		}

		ctx, cancel := context.WithTimeout(parent, 15*time.Second)
		defer cancel()

		firewallMu.Lock()
		defer firewallMu.Unlock()

		if err := pruneFirewallRenewalLeasesLocked(); err != nil {
			logger.Debug("certificate firewall lease recovery failed:", err)
		}
		if err := migrateManagedBackendIfNeededLocked(ctx); err != nil {
			logger.Debug("firewall backend migration failed:", err)
			return
		}
		backend, err := detectManagedFirewallBackend(ctx)
		if err != nil {
			return
		}
		desiredEnabled, configured, err := firewallManagedEnabledPreference()
		if err != nil {
			logger.Debug("firewall desired state read failed:", err)
			return
		}
		on, err := backend.enabled(ctx)
		if err != nil {
			return
		}
		if !configured && on && isManagedNativeFirewall(backend.name) {
			if err := setFirewallManagedEnabledPreference(true); err != nil {
				logger.Debug("firewall desired state migration failed:", err)
				return
			}
			desiredEnabled = true
			configured = true
		}

		// Once the panel toggle has an explicit value, a disabled managed
		// firewall must stay disabled even if UFW/firewalld themselves are
		// still running for administrator-owned rules.
		if configured && !desiredEnabled {
			if err := s.reconcileManagedPingStateLocked(false); err != nil {
				logger.Debug("firewall ping reconcile failed:", err)
			}
			return
		}

		if shouldRestoreManagedNativeFirewall(backend.name, on, desiredEnabled) {
			if err := s.syncManagedSafeLocked(ctx, backend, rememberedFirewallSafetyPort()); err != nil {
				logger.Debug("firewall native restore failed:", err)
				return
			}
			on = true
		}
		if err := s.reconcileManagedPingStateLocked(on); err != nil {
			logger.Debug("firewall ping reconcile failed:", err)
		}
		if !on {
			return
		}
		auto, err := firewallAutoSync()
		if err != nil || (!auto && !explicit && !needsCertificateFirewallReconcile()) {
			return
		}
		if err := s.syncManagedSafeLocked(ctx, backend, rememberedFirewallSafetyPort()); err != nil {
			logger.Debug("firewall auto-sync failed:", err)
		}
	}

	syncNow(false)
	timer := time.NewTimer(firewallSyncInterval())
	defer timer.Stop()
	for {
		explicit := false
		select {
		case <-parent.Done():
			return
		case <-timer.C:
		case <-firewallSyncTrigger:
			explicit = true
		}
		syncNow(explicit)
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(firewallSyncInterval())
	}
}
