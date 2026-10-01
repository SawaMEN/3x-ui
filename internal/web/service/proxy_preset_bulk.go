package service

import (
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func normalizeGroupIDs(groupIDs []string) []string {
	seen := make(map[string]struct{}, len(groupIDs))
	out := make([]string, 0, len(groupIDs))
	for _, raw := range groupIDs {
		groupID := strings.TrimSpace(raw)
		if groupID == "" {
			continue
		}
		if _, ok := seen[groupID]; ok {
			continue
		}
		seen[groupID] = struct{}{}
		out = append(out, groupID)
	}
	sort.Strings(out)
	return out
}

func (s *ProxyPresetService) AssignMany(userID int, groupIDs []string, presetID *int) ([]ProxyPresetAssignment, error) {
	if err := ensureProxyPresetSchema(); err != nil {
		return nil, err
	}
	groupIDs = normalizeGroupIDs(groupIDs)
	if len(groupIDs) == 0 {
		return nil, errors.New("at least one groupId is required")
	}
	db := database.GetDB()
	var preset model.ProxyPreset
	if presetID != nil {
		if *presetID <= 0 {
			return nil, errors.New("presetId must be a positive integer")
		}
		if err := db.Where("user_id = ? AND id = ?", userID, *presetID).First(&preset).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("proxy preset not found")
			}
			return nil, err
		}
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		for _, groupID := range groupIDs {
			belongs, err := hostGroupBelongsToUser(tx, userID, groupID)
			if err != nil {
				return err
			}
			if !belongs {
				return errors.New("host group not found")
			}
		}
		for _, groupID := range groupIDs {
			if presetID == nil {
				if err := tx.Where("user_id = ? AND group_id = ?", userID, groupID).Delete(&model.HostProxyPreset{}).Error; err != nil {
					return err
				}
				continue
			}
			now := time.Now()
			binding := model.HostProxyPreset{GroupId: groupID, UserId: userID, PresetId: *presetID, CreatedAt: now, UpdatedAt: now}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{{Name: "group_id"}},
				DoUpdates: clause.Assignments(map[string]any{"user_id": userID, "preset_id": *presetID, "updated_at": now}),
			}).Create(&binding).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	model.InvalidateProxyPresetCache()
	if presetID == nil {
		return []ProxyPresetAssignment{}, nil
	}
	out := make([]ProxyPresetAssignment, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		out = append(out, ProxyPresetAssignment{GroupId: groupID, PresetId: preset.Id, PresetName: preset.Name})
	}
	return out, nil
}
