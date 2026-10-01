package service

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/random"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const proxyBundleVersion = 1

type ProxyBundleHost struct {
	entity.HostGroup
	InboundTags []string `json:"inboundTags"`
	PresetName  string   `json:"presetName,omitempty"`
}

type ProxyBundle struct {
	Version    int                `json:"version"`
	ExportedAt int64              `json:"exportedAt"`
	Presets    []ProxyPresetInput `json:"presets"`
	Hosts      []ProxyBundleHost  `json:"hosts"`
}

type ProxyBundleImportRequest struct {
	Bundle               ProxyBundle `json:"bundle"`
	DryRun               bool        `json:"dryRun"`
	AllowMissingInbounds bool        `json:"allowMissingInbounds"`
}

type ProxyBundleImportResult struct {
	DryRun             bool     `json:"dryRun"`
	CreatedPresets     int      `json:"createdPresets"`
	UpdatedPresets     int      `json:"updatedPresets"`
	CreatedHosts       int      `json:"createdHosts"`
	UpdatedHosts       int      `json:"updatedHosts"`
	AssignedPresets    int      `json:"assignedPresets"`
	SkippedHosts       []string `json:"skippedHosts"`
	MissingInboundTags []string `json:"missingInboundTags"`
	MissingPresetNames []string `json:"missingPresetNames"`
}

func (s *ProxyPresetService) ExportBundle(userID int) (*ProxyBundle, error) {
	if err := ensureProxyPresetSchema(); err != nil {
		return nil, err
	}
	db := database.GetDB()
	var inbounds []model.Inbound
	if err := db.Where("user_id = ?", userID).Order("id ASC").Find(&inbounds).Error; err != nil {
		return nil, err
	}
	tagByID := make(map[int]string, len(inbounds))
	for _, inbound := range inbounds {
		tagByID[inbound.Id] = inbound.Tag
	}

	var hosts []*model.Host
	if err := db.Table("hosts AS h").Select("h.*").Joins("JOIN inbounds AS i ON i.id = h.inbound_id").
		Where("i.user_id = ?", userID).Order("h.sort_order ASC, h.id ASC").Scan(&hosts).Error; err != nil {
		return nil, err
	}
	groups := groupHosts(hosts)

	var presets []model.ProxyPreset
	if err := db.Where("user_id = ?", userID).Order("name ASC, id ASC").Find(&presets).Error; err != nil {
		return nil, err
	}
	presetNameByID := make(map[int]string, len(presets))
	bundlePresets := make([]ProxyPresetInput, 0, len(presets))
	for i := range presets {
		view, err := decodeProxyPreset(&presets[i])
		if err != nil {
			return nil, err
		}
		presetNameByID[view.Id] = view.Name
		bundlePresets = append(bundlePresets, ProxyPresetInput{Name: view.Name, Description: view.Description, Config: view.Config})
	}

	var bindings []model.HostProxyPreset
	if err := db.Where("user_id = ?", userID).Find(&bindings).Error; err != nil {
		return nil, err
	}
	presetByGroup := make(map[string]string, len(bindings))
	for _, binding := range bindings {
		if name := presetNameByID[binding.PresetId]; name != "" {
			presetByGroup[binding.GroupId] = name
		}
	}

	bundleHosts := make([]ProxyBundleHost, 0, len(groups))
	for _, group := range groups {
		copyGroup := *group
		copyGroup.InboundIds = nil
		tags := make([]string, 0, len(group.InboundIds))
		for _, inboundID := range group.InboundIds {
			if tag := tagByID[inboundID]; tag != "" {
				tags = append(tags, tag)
			}
		}
		sort.Strings(tags)
		bundleHosts = append(bundleHosts, ProxyBundleHost{
			HostGroup:   copyGroup,
			InboundTags: tags,
			PresetName:  presetByGroup[group.GroupId],
		})
	}

	return &ProxyBundle{
		Version: proxyBundleVersion, ExportedAt: time.Now().Unix(), Presets: bundlePresets, Hosts: bundleHosts,
	}, nil
}

func resolveBundleInboundIDs(inbounds map[string]int, tags []string) ([]int, []string) {
	ids := make([]int, 0, len(tags))
	missing := make([]string, 0)
	seenIDs := make(map[int]struct{}, len(tags))
	seenMissing := make(map[string]struct{}, len(tags))
	for _, raw := range tags {
		tag := strings.TrimSpace(raw)
		if tag == "" {
			continue
		}
		id, ok := inbounds[tag]
		if !ok {
			if _, seen := seenMissing[tag]; !seen {
				missing = append(missing, tag)
				seenMissing[tag] = struct{}{}
			}
			continue
		}
		if _, seen := seenIDs[id]; seen {
			continue
		}
		seenIDs[id] = struct{}{}
		ids = append(ids, id)
	}
	sort.Ints(ids)
	sort.Strings(missing)
	return ids, missing
}

type preparedBundleHost struct {
	item     ProxyBundleHost
	group    entity.HostGroup
	existing bool
	groupID  string
	skip     bool
}

type preparedProxyBundle struct {
	presets         []ProxyPresetInput
	hosts           []preparedBundleHost
	missingInbounds []string
	missingPresets  []string
}

func prepareProxyBundleImport(userID int, req ProxyBundleImportRequest) (*preparedProxyBundle, *ProxyBundleImportResult, error) {
	if req.Bundle.Version != proxyBundleVersion {
		return nil, nil, fmt.Errorf("unsupported proxy bundle version %d", req.Bundle.Version)
	}
	db := database.GetDB()
	var inbounds []model.Inbound
	if err := db.Where("user_id = ?", userID).Find(&inbounds).Error; err != nil {
		return nil, nil, err
	}
	inboundByTag := make(map[string]int, len(inbounds))
	for _, inbound := range inbounds {
		if inbound.Tag != "" {
			inboundByTag[inbound.Tag] = inbound.Id
		}
	}

	var existingPresets []model.ProxyPreset
	if err := db.Where("user_id = ?", userID).Order("id ASC").Find(&existingPresets).Error; err != nil {
		return nil, nil, err
	}
	presetExists := make(map[string]bool, len(existingPresets)+len(req.Bundle.Presets))
	for _, preset := range existingPresets {
		presetExists[preset.Name] = true
	}
	result := &ProxyBundleImportResult{DryRun: req.DryRun, SkippedHosts: []string{}, MissingInboundTags: []string{}, MissingPresetNames: []string{}}
	prepared := &preparedProxyBundle{}
	seenPresetNames := make(map[string]struct{}, len(req.Bundle.Presets))
	for _, incoming := range req.Bundle.Presets {
		candidate := incoming
		if err := validateProxyPresetInput(&candidate); err != nil {
			return nil, nil, fmt.Errorf("preset %q: %w", incoming.Name, err)
		}
		if _, duplicate := seenPresetNames[candidate.Name]; duplicate {
			return nil, nil, fmt.Errorf("duplicate preset name %q in bundle", candidate.Name)
		}
		seenPresetNames[candidate.Name] = struct{}{}
		if presetExists[candidate.Name] {
			result.UpdatedPresets++
		} else {
			result.CreatedPresets++
		}
		presetExists[candidate.Name] = true
		prepared.presets = append(prepared.presets, candidate)
	}
	if len(existingPresets)+result.CreatedPresets > proxyPresetMaxCount {
		return nil, nil, fmt.Errorf("proxy preset limit reached: maximum is %d", proxyPresetMaxCount)
	}

	validate := validator.New(validator.WithRequiredStructEnabled())
	missingInboundSet := map[string]struct{}{}
	missingPresetSet := map[string]struct{}{}
	skippedSet := map[string]struct{}{}
	seenGroupIDs := map[string]struct{}{}
	for _, incoming := range req.Bundle.Hosts {
		item := incoming
		item.PresetName = strings.TrimSpace(item.PresetName)
		group := item.HostGroup
		group.GroupId = strings.TrimSpace(group.GroupId)
		if group.GroupId != "" {
			if _, duplicate := seenGroupIDs[group.GroupId]; duplicate {
				return nil, nil, fmt.Errorf("duplicate host groupId %q in bundle", group.GroupId)
			}
			seenGroupIDs[group.GroupId] = struct{}{}
		}
		ids, missing := resolveBundleInboundIDs(inboundByTag, item.InboundTags)
		group.InboundIds = ids
		for _, tag := range missing {
			missingInboundSet[tag] = struct{}{}
		}
		entry := preparedBundleHost{item: item, group: group, groupID: group.GroupId}
		if len(missing) > 0 || len(ids) == 0 {
			entry.skip = true
		}
		if err := validate.Struct(group); err != nil && !entry.skip {
			return nil, nil, fmt.Errorf("host group %q: %w", group.Remark, err)
		}
		if item.PresetName != "" && !presetExists[item.PresetName] {
			missingPresetSet[item.PresetName] = struct{}{}
			entry.skip = true
		}
		if entry.groupID != "" {
			owned, err := hostGroupBelongsToUser(db, userID, entry.groupID)
			if err != nil {
				return nil, nil, err
			}
			entry.existing = owned
		}
		if entry.skip {
			label := strings.TrimSpace(group.Remark)
			if label == "" {
				label = entry.groupID
			}
			if _, seen := skippedSet[label]; !seen {
				result.SkippedHosts = append(result.SkippedHosts, label)
				skippedSet[label] = struct{}{}
			}
		} else {
			if entry.existing {
				result.UpdatedHosts++
			} else {
				result.CreatedHosts++
			}
			if item.PresetName != "" {
				result.AssignedPresets++
			}
		}
		prepared.hosts = append(prepared.hosts, entry)
	}
	for tag := range missingInboundSet {
		result.MissingInboundTags = append(result.MissingInboundTags, tag)
	}
	for name := range missingPresetSet {
		result.MissingPresetNames = append(result.MissingPresetNames, name)
	}
	sort.Strings(result.SkippedHosts)
	sort.Strings(result.MissingInboundTags)
	sort.Strings(result.MissingPresetNames)
	prepared.missingInbounds = result.MissingInboundTags
	prepared.missingPresets = result.MissingPresetNames
	return prepared, result, nil
}

func (s *ProxyPresetService) ImportBundle(userID int, req ProxyBundleImportRequest) (*ProxyBundleImportResult, error) {
	if err := ensureProxyPresetSchema(); err != nil {
		return nil, err
	}
	prepared, result, err := prepareProxyBundleImport(userID, req)
	if err != nil {
		return nil, err
	}
	if req.DryRun {
		return result, nil
	}
	if len(prepared.missingPresets) > 0 {
		return nil, fmt.Errorf("bundle references missing presets: %s", strings.Join(prepared.missingPresets, ", "))
	}
	if len(prepared.missingInbounds) > 0 && !req.AllowMissingInbounds {
		return nil, fmt.Errorf("bundle references missing inbound tags: %s", strings.Join(prepared.missingInbounds, ", "))
	}

	db := database.GetDB()
	err = db.Transaction(func(tx *gorm.DB) error {
		presetIDs := make(map[string]int)
		for _, incoming := range prepared.presets {
			config, err := encodeProxyPresetConfig(incoming.Config)
			if err != nil {
				return err
			}
			var row model.ProxyPreset
			err = tx.Where("user_id = ? AND name = ?", userID, incoming.Name).Order("id ASC").First(&row).Error
			switch {
			case err == nil:
				row.Description = incoming.Description
				row.Config = config
				row.UpdatedAt = time.Now()
				if err := tx.Save(&row).Error; err != nil {
					return err
				}
			case errors.Is(err, gorm.ErrRecordNotFound):
				now := time.Now()
				row = model.ProxyPreset{UserId: userID, Name: incoming.Name, Description: incoming.Description, Config: config, CreatedAt: now, UpdatedAt: now}
				if err := tx.Create(&row).Error; err != nil {
					return err
				}
			default:
				return err
			}
			presetIDs[incoming.Name] = row.Id
		}
		var existingPresets []model.ProxyPreset
		if err := tx.Where("user_id = ?", userID).Find(&existingPresets).Error; err != nil {
			return err
		}
		for _, row := range existingPresets {
			if _, ok := presetIDs[row.Name]; !ok {
				presetIDs[row.Name] = row.Id
			}
		}

		for _, entry := range prepared.hosts {
			if entry.skip {
				if req.AllowMissingInbounds {
					continue
				}
				return fmt.Errorf("host group %q cannot be imported", entry.group.Remark)
			}
			groupID := entry.groupID
			if groupID == "" {
				groupID = random.NumLower(16)
			} else if !entry.existing {
				var count int64
				if err := tx.Model(&model.Host{}).Where("group_id = ?", groupID).Count(&count).Error; err != nil {
					return err
				}
				if count > 0 {
					groupID = random.NumLower(16)
				}
			}
			if err := validateInboundsExist(tx, entry.group.InboundIds); err != nil {
				return err
			}
			if entry.existing {
				ownedInbounds := tx.Model(&model.Inbound{}).Select("id").Where("user_id = ?", userID)
				if err := tx.Where("group_id = ? AND inbound_id IN (?)", groupID, ownedInbounds).Delete(&model.Host{}).Error; err != nil {
					return err
				}
			}
			rows := buildHostRows(groupID, &entry.group)
			if len(rows) > 0 {
				if err := tx.Create(&rows).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("user_id = ? AND group_id = ?", userID, groupID).Delete(&model.HostProxyPreset{}).Error; err != nil {
				return err
			}
			if entry.item.PresetName != "" {
				presetID := presetIDs[entry.item.PresetName]
				now := time.Now()
				binding := model.HostProxyPreset{GroupId: groupID, UserId: userID, PresetId: presetID, CreatedAt: now, UpdatedAt: now}
				if err := tx.Clauses(clause.OnConflict{
					Columns:   []clause.Column{{Name: "group_id"}},
					DoUpdates: clause.Assignments(map[string]any{"user_id": userID, "preset_id": presetID, "updated_at": now}),
				}).Create(&binding).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	model.InvalidateProxyPresetCache()
	return result, nil
}
