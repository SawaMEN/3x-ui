package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/entity"
	"gorm.io/gorm"
)

type ProxyPresetPreviewRequest struct {
	GroupIds []string                 `json:"groupIds"`
	PresetId *int                     `json:"presetId"`
	Config   *model.ProxyPresetConfig `json:"config"`
}

type ProxyPresetPreviewItem struct {
	GroupId       string            `json:"groupId"`
	Remark        string            `json:"remark"`
	Base          *entity.HostGroup `json:"base"`
	Effective     *entity.HostGroup `json:"effective"`
	ChangedFields []string          `json:"changedFields"`
	Warnings      []string          `json:"warnings"`
}

var proxyPresetPreviewFields = []string{
	"port", "serverDescription", "security", "sni", "hostHeader", "path", "alpn",
	"fingerprint", "cipherSuites", "overrideSniFromAddress", "keepSniBlank",
	"pinnedPeerCertSha256", "verifyPeerCertByName", "allowInsecure", "echConfigList",
	"muxParams", "sockoptParams", "finalMask", "vlessRoute", "excludeFromSubTypes",
	"mihomoIpVersion", "mihomoX25519", "shuffleHost",
}

func previewChangedFields(base, effective *entity.HostGroup) []string {
	baseJSON, _ := json.Marshal(base)
	effectiveJSON, _ := json.Marshal(effective)
	var baseMap map[string]any
	var effectiveMap map[string]any
	_ = json.Unmarshal(baseJSON, &baseMap)
	_ = json.Unmarshal(effectiveJSON, &effectiveMap)
	changed := make([]string, 0)
	for _, field := range proxyPresetPreviewFields {
		if !reflect.DeepEqual(baseMap[field], effectiveMap[field]) {
			changed = append(changed, field)
		}
	}
	return changed
}

func effectiveHostGroup(base *entity.HostGroup, cfg model.ProxyPresetConfig) *entity.HostGroup {
	if base == nil {
		return nil
	}
	rows := buildHostRows(base.GroupId, base)
	if len(rows) == 0 {
		clone := *base
		return &clone
	}
	sample := *rows[0]
	model.ApplyProxyPresetConfig(&sample, cfg)
	effective := newHostGroup(&sample, base.GroupId)
	effective.InboundIds = append([]int(nil), base.InboundIds...)
	effective.Hosts = append([]string(nil), base.Hosts...)
	return effective
}

func appendPreviewWarning(warnings []string, value string) []string {
	for _, existing := range warnings {
		if existing == value {
			return warnings
		}
	}
	return append(warnings, value)
}

func proxyPresetPreviewWarnings(db *gorm.DB, userID int, effective *entity.HostGroup) ([]string, error) {
	warnings := make([]string, 0)
	if effective.Security == "reality" {
		warnings = appendPreviewWarning(warnings, "Reality parameters are inherited from the inbound; a Host preset cannot convert a non-Reality inbound to Reality")
	}
	if effective.KeepSniBlank && (effective.Sni != "" || effective.OverrideSniFromAddress) {
		warnings = appendPreviewWarning(warnings, "keepSniBlank suppresses the configured SNI override")
	}
	if effective.Security == "none" && effective.AllowInsecure {
		warnings = appendPreviewWarning(warnings, "allowInsecure has no effect when security is none")
	}
	if len(effective.ExcludeFromSubTypes) >= 3 {
		seen := map[string]bool{}
		for _, format := range effective.ExcludeFromSubTypes {
			seen[format] = true
		}
		if seen["raw"] && seen["json"] && seen["clash"] {
			warnings = appendPreviewWarning(warnings, "Host is excluded from raw, JSON and Clash subscriptions")
		}
	}

	if effective.HostHeader == "" && effective.Path == "" {
		sort.Strings(warnings)
		return warnings, nil
	}
	var inbounds []model.Inbound
	if err := db.Where("user_id = ? AND id IN ?", userID, effective.InboundIds).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		var stream map[string]any
		if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
			warnings = appendPreviewWarning(warnings, fmt.Sprintf("Inbound %s has invalid streamSettings JSON", inbound.Tag))
			continue
		}
		network, _ := stream["network"].(string)
		supportsPath := network == "ws" || network == "httpupgrade" || network == "xhttp"
		if effective.Path != "" && !supportsPath {
			warnings = appendPreviewWarning(warnings, fmt.Sprintf("path has no effect on inbound %s transport %s", inbound.Tag, network))
		}
		if effective.HostHeader != "" && !supportsPath {
			warnings = appendPreviewWarning(warnings, fmt.Sprintf("hostHeader has no effect on inbound %s transport %s", inbound.Tag, network))
		}
	}
	sort.Strings(warnings)
	return warnings, nil
}

func (s *ProxyPresetService) Preview(userID int, req ProxyPresetPreviewRequest) ([]ProxyPresetPreviewItem, error) {
	if err := ensureProxyPresetSchema(); err != nil {
		return nil, err
	}
	groupIDs := normalizeGroupIDs(req.GroupIds)
	if len(groupIDs) == 0 {
		return nil, errors.New("at least one groupId is required")
	}
	if (req.PresetId == nil) == (req.Config == nil) {
		return nil, errors.New("provide exactly one of presetId or config")
	}

	var cfg model.ProxyPresetConfig
	if req.PresetId != nil {
		if *req.PresetId <= 0 {
			return nil, errors.New("presetId must be a positive integer")
		}
		var row model.ProxyPreset
		if err := database.GetDB().Where("user_id = ? AND id = ?", userID, *req.PresetId).First(&row).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, errors.New("proxy preset not found")
			}
			return nil, err
		}
		if err := json.Unmarshal([]byte(row.Config), &cfg); err != nil {
			return nil, err
		}
	} else {
		candidate := ProxyPresetInput{Name: "preview", Config: *req.Config}
		if err := validateProxyPresetInput(&candidate); err != nil {
			return nil, err
		}
		cfg = candidate.Config
	}

	db := database.GetDB()
	hostService := HostService{}
	out := make([]ProxyPresetPreviewItem, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		belongs, err := hostGroupBelongsToUser(db, userID, groupID)
		if err != nil {
			return nil, err
		}
		if !belongs {
			return nil, errors.New("host group not found")
		}
		base, err := hostService.GetHostGroup(groupID)
		if err != nil {
			return nil, err
		}
		effective := effectiveHostGroup(base, cfg)
		warnings, err := proxyPresetPreviewWarnings(db, userID, effective)
		if err != nil {
			return nil, err
		}
		out = append(out, ProxyPresetPreviewItem{
			GroupId:       groupID,
			Remark:        strings.TrimSpace(base.Remark),
			Base:          base,
			Effective:     effective,
			ChangedFields: previewChangedFields(base, effective),
			Warnings:      warnings,
		})
	}
	return out, nil
}
