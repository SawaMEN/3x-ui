package service

import (
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// ReconcileSubscriptionProxies makes the deterministic Telemt accounts match
// every subscription already stored by the panel. Enabling provisions missing
// accounts for all saved subscription IDs; disabling revokes all of them.
//
// The operation is intentionally idempotent so it can also be used to repair a
// partially-applied previous attempt without creating duplicate Telemt users.
func (TelemtService) ReconcileSubscriptionProxies(enabled bool, host string) error {
	var subIDs []string
	if err := database.GetDB().
		Model(&model.ClientRecord{}).
		Where("TRIM(sub_id) <> ?", "").
		Distinct("sub_id").
		Order("sub_id ASC").
		Pluck("sub_id", &subIDs).Error; err != nil {
		return fmt.Errorf("telemt: list saved subscriptions: %w", err)
	}

	if enabled {
		host = strings.TrimSpace(host)
		if host == "" {
			return fmt.Errorf("telemt: public host is required to enable subscription proxies")
		}
	}

	failed := 0
	var firstErr error
	for _, subID := range subIDs {
		var err error
		if enabled {
			_, err = (TelemtService{}).EnsureSubscriptionProxy(subID, host)
		} else {
			err = (TelemtService{}).DeleteSubscriptionProxy(subID)
		}
		if err == nil {
			continue
		}
		failed++
		if firstErr == nil {
			firstErr = err
		}
	}

	if failed == 0 {
		return nil
	}
	action := "create"
	if !enabled {
		action = "delete"
	}
	return fmt.Errorf("telemt: failed to %s subscription proxies for %d of %d saved subscriptions: %w", action, failed, len(subIDs), firstErr)
}
