package sub

import (
	"encoding/json"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"gorm.io/gorm/clause"
)

// repairSubscriptionBindings restores missing client_inbounds rows left by
// legacy imports or partial migrations. A binding is repaired only when the
// normalized client belongs to subID and the same inbound still contains a
// settings.clients entry with the same email and subId. This keeps repair
// narrowly scoped to an already-established subscription identity and avoids
// attaching unrelated inbounds merely because an email happens to match.
func (s *SubService) repairSubscriptionBindings(subID string) (int, error) {
	subID = strings.TrimSpace(subID)
	if subID == "" || database.GetDB() == nil {
		return 0, nil
	}

	db := database.GetDB()
	var records []model.ClientRecord
	if err := db.Where("sub_id = ?", subID).Order("id ASC").Find(&records).Error; err != nil {
		return 0, err
	}
	if len(records) == 0 {
		return 0, nil
	}

	// Prefer an exact email match. A case-insensitive fallback is used only
	// when it is unambiguous, matching subscription lookup's legacy behavior
	// without cross-linking case-distinct imported identities.
	exact := make(map[string]model.ClientRecord, len(records))
	folded := make(map[string]model.ClientRecord, len(records))
	ambiguous := make(map[string]bool)
	clientIDs := make([]int, 0, len(records))
	for _, record := range records {
		exact[record.Email] = record
		key := strings.ToLower(strings.TrimSpace(record.Email))
		if key != "" {
			if _, exists := folded[key]; exists {
				ambiguous[key] = true
			} else {
				folded[key] = record
			}
		}
		clientIDs = append(clientIDs, record.Id)
	}

	var existing []model.ClientInbound
	if err := db.Where("client_id IN ?", clientIDs).Find(&existing).Error; err != nil {
		return 0, err
	}
	type bindingKey struct {
		clientID  int
		inboundID int
	}
	seen := make(map[bindingKey]struct{}, len(existing))
	for _, binding := range existing {
		seen[bindingKey{clientID: binding.ClientId, inboundID: binding.InboundId}] = struct{}{}
	}

	// Keep this query dialect-neutral: subscriptions are supported on SQLite
	// and PostgreSQL. Only id + settings are needed for repair, so even a panel
	// with many inbounds avoids loading traffic/stream/config columns here.
	var inbounds []model.Inbound
	if err := db.Select("id", "settings").Where("enable = ?", true).Order("id ASC").Find(&inbounds).Error; err != nil {
		return 0, err
	}

	toCreate := make([]model.ClientInbound, 0)
	for i := range inbounds {
		var settings struct {
			Clients []struct {
				Email string `json:"email"`
				SubID string `json:"subId"`
			} `json:"clients"`
		}
		if err := json.Unmarshal([]byte(inbounds[i].Settings), &settings); err != nil {
			continue
		}
		for _, embedded := range settings.Clients {
			if strings.TrimSpace(embedded.SubID) != subID || strings.TrimSpace(embedded.Email) == "" {
				continue
			}

			record, ok := exact[embedded.Email]
			if !ok {
				key := strings.ToLower(strings.TrimSpace(embedded.Email))
				if key == "" || ambiguous[key] {
					continue
				}
				record, ok = folded[key]
				if !ok {
					continue
				}
			}

			key := bindingKey{clientID: record.Id, inboundID: inbounds[i].Id}
			if _, exists := seen[key]; exists {
				continue
			}
			seen[key] = struct{}{}
			toCreate = append(toCreate, model.ClientInbound{
				ClientId:  record.Id,
				InboundId: inbounds[i].Id,
			})
		}
	}

	if len(toCreate) == 0 {
		return 0, nil
	}
	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&toCreate)
	if result.Error != nil {
		return 0, result.Error
	}
	return int(result.RowsAffected), nil
}
