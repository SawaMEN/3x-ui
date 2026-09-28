package service

import (
	"context"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

var firewallAutoSyncOnce sync.Once

// StartAutoSync keeps firewall rules aligned with enabled local inbounds even
// when changes arrive through imports, API calls, or node synchronization
// rather than the browser that opened the firewall dialog. The fast interval is
// deliberately cheap while the firewall is disabled: SyncIfEnabled only reads
// the setting/backend state and returns without mutating rules.
func (s *FirewallService) StartAutoSync() {
	firewallAutoSyncOnce.Do(func() {
		go func() {
			syncNow := func() {
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
