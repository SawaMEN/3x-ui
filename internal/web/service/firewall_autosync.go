package service

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

const firewallControlInitializedKey = "firewallControlInitialized"

var firewallAutoSyncOnce sync.Once

func (s *FirewallService) MarkControlInitialized() error {
	return (&SettingService{}).setBool(firewallControlInitializedKey, true)
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

// StartAutoSync keeps firewall rules aligned with enabled local inbounds even
// when changes arrive through imports, API calls, or node synchronization
// rather than the browser that opened the firewall dialog. Existing system
// firewall installations are not adopted automatically after an upgrade: the
// background reconciler starts mutating rules only after the operator performs
// a firewall action from the panel at least once.
func (s *FirewallService) StartAutoSync() {
	firewallAutoSyncOnce.Do(func() {
		go func() {
			syncNow := func() {
				if !firewallControlInitialized() {
					return
				}
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if err := s.SyncIfEnabled(ctx); err != nil {
					logger.Debug("firewall auto-sync failed:", err)
				}
			}

			syncNow()
			ticker := time.NewTicker(5 * time.Second)
			defer ticker.Stop()
			for range ticker.C {
				syncNow()
			}
		}()
	})
}
