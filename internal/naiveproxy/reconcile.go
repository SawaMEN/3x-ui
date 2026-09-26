package naiveproxy

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

// reconcileStandalone builds the Xray-side Naive server from the normalized
// client tables. The embedded settings.clients array remains a compatibility
// fallback for pre-migration rows and for credentials that have not yet been
// copied into clients.password.
func reconcileStandalone(ctx context.Context) error {
	db := database.GetDB()
	if db == nil {
		return nil
	}

	coreType := "xray"
	var setting struct{ Value string }
	if err := db.Table("settings").Select("value").Where("key = ?", "coreType").Take(&setting).Error; err == nil && strings.TrimSpace(setting.Value) != "" {
		coreType = strings.TrimSpace(setting.Value)
	}
	if coreType != "xray" {
		return Stop()
	}

	var rows []*model.Inbound
	if err := db.Model(&model.Inbound{}).
		Preload("ClientStats").
		Where("protocol = ? AND enable = ? AND node_id IS NULL", model.NaiveProxy, true).
		Order("id ASC").
		Find(&rows).Error; err != nil {
		return err
	}

	inbounds := make([]Inbound, 0, len(rows))
	for _, row := range rows {
		var settings inboundSettings
		if err := json.Unmarshal([]byte(row.Settings), &settings); err != nil {
			return fmt.Errorf("Naive inbound %q has invalid settings: %w", row.Tag, err)
		}

		var records []model.ClientRecord
		if err := db.Model(&model.ClientRecord{}).
			Joins("JOIN client_inbounds ON client_inbounds.client_id = clients.id").
			Where("client_inbounds.inbound_id = ?", row.Id).
			Order("clients.id ASC").
			Find(&records).Error; err != nil {
			return fmt.Errorf("load Naive clients for inbound %q: %w", row.Tag, err)
		}

		users := mergeNaiveUsers(records, settings.Clients, row.ClientStats)
		// An enabled inbound with no usable credentials must not keep the
		// previously rendered Caddy site alive. Treat it as inactive so Sync
		// removes it, and stops the sidecar entirely when no active sites remain.
		if len(users) == 0 {
			continue
		}
		inbounds = append(inbounds, Inbound{
			Tag:             row.Tag,
			Listen:          row.Listen,
			Port:            row.Port,
			CertificatePath: strings.TrimSpace(settings.TLS.CertificatePath),
			KeyPath:         strings.TrimSpace(settings.TLS.KeyPath),
			Users:           users,
		})
	}
	return Sync(ctx, inbounds)
}

func mergeNaiveUsers(records []model.ClientRecord, legacy []model.Client, stats []xray.ClientTraffic) []User {
	enabledByEmail := make(map[string]bool, len(stats))
	for _, stat := range stats {
		enabledByEmail[strings.ToLower(strings.TrimSpace(stat.Email))] = stat.Enable
	}

	legacyByEmail := make(map[string]model.Client, len(legacy))
	for _, client := range legacy {
		key := strings.ToLower(strings.TrimSpace(client.Email))
		if key != "" {
			if _, exists := legacyByEmail[key]; !exists {
				legacyByEmail[key] = client
			}
		}
	}

	users := make([]User, 0, len(records)+len(legacy))
	seen := make(map[string]struct{}, len(records)+len(legacy))
	appendUser := func(username, password string, enabled bool) {
		username = strings.TrimSpace(username)
		key := strings.ToLower(username)
		if key == "" {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		if !enabled || password == "" {
			return
		}
		if statEnabled, exists := enabledByEmail[key]; exists && !statEnabled {
			return
		}
		users = append(users, User{Username: username, Password: password})
	}

	for _, record := range records {
		username := strings.TrimSpace(record.Email)
		key := strings.ToLower(username)
		password := record.Password
		if password == "" {
			if fallback, exists := legacyByEmail[key]; exists {
				password = fallback.Password
			}
		}
		appendUser(username, password, record.Enable)
	}

	for _, client := range legacy {
		appendUser(client.Email, client.Password, client.Enable)
	}
	return users
}
