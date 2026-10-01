package service

import (
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func (s *ProxyPresetService) Assignments(userID int) ([]ProxyPresetAssignment, error) {
	if err := ensureProxyPresetSchema(); err != nil {
		return nil, err
	}
	var bindings []model.HostProxyPreset
	if err := database.GetDB().Where("user_id = ?", userID).Order("group_id ASC").Find(&bindings).Error; err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return []ProxyPresetAssignment{}, nil
	}
	ids := make([]int, 0, len(bindings))
	seen := make(map[int]struct{}, len(bindings))
	for _, binding := range bindings {
		if _, ok := seen[binding.PresetId]; ok {
			continue
		}
		seen[binding.PresetId] = struct{}{}
		ids = append(ids, binding.PresetId)
	}
	var presets []model.ProxyPreset
	if err := database.GetDB().Where("user_id = ? AND id IN ?", userID, ids).Find(&presets).Error; err != nil {
		return nil, err
	}
	nameByID := make(map[int]string, len(presets))
	for _, preset := range presets {
		nameByID[preset.Id] = preset.Name
	}
	out := make([]ProxyPresetAssignment, 0, len(bindings))
	for _, binding := range bindings {
		name, ok := nameByID[binding.PresetId]
		if !ok {
			continue
		}
		out = append(out, ProxyPresetAssignment{GroupId: binding.GroupId, PresetId: binding.PresetId, PresetName: name})
	}
	return out, nil
}
