package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// Replace only connection references, never DNS names or arbitrary text.
func replaceConnectionTags(value any, removed map[string]bool, replacement, parent string) bool {
	changed := false
	switch node := value.(type) {
	case []any:
		for _, v := range node {
			changed = replaceConnectionTags(v, removed, replacement, parent) || changed
		}
	case map[string]any:
		for key, v := range node {
			if key == "headers" {
				continue
			}
			reference := key == "outbound" || key == "outboundTag" || key == "detour" || key == "download_detour" || key == "portal_tag" || key == "bridge_tag" || key == "dialerProxy" || key == "fallbackTag" || key == "proxyTag" || key == "tag" && parent == "proxySettings" || key == "final" && parent == "route" || key == "default" && (node["type"] == "selector" || node["type"] == "urltest" || parent == "settings" && node["outbounds"] != nil)
			if text, ok := v.(string); ok && reference && removed[text] {
				node[key] = replacement
				changed = true
				continue
			}
			if key == "outbounds" || key == "selector" {
				if list, ok := v.([]any); ok {
					members := make([]any, 0, len(list))
					seen := map[string]bool{}
					for _, item := range list {
						if tag, ok := item.(string); ok && removed[tag] {
							item = replacement
							changed = true
						}
						if tag, ok := item.(string); ok {
							if seen[tag] {
								continue
							}
							seen[tag] = true
						}
						members = append(members, item)
					}
					node[key] = members
					v = members
				}
			}
			changed = replaceConnectionTags(v, removed, replacement, key) || changed
		}
	}
	return changed
}

func deleteSubscriptionAndReferences(tx *gorm.DB, sub *model.OutboundSubscription, replacement string) error {
	removed := map[string]bool{autoOutboundTag(sub.Id): true, autoBlockedTag(sub.Id): true}
	var nodes []map[string]any
	if strings.TrimSpace(sub.LastFetchedOutbounds) != "" {
		if err := json.Unmarshal([]byte(sub.LastFetchedOutbounds), &nodes); err != nil {
			return err
		}
	}
	for _, node := range nodes {
		if tag, ok := node["tag"].(string); ok {
			removed[tag] = true
		}
	}
	var settings []model.Setting
	if err := tx.Where("key IN ?", []string{"xrayTemplateConfig", "singBoxConfigTemplate", "singBoxUpstreamConfigTemplate"}).Find(&settings).Error; err != nil {
		return err
	}
	available := map[string]bool{}
	hasXrayTemplate := false
	for _, row := range settings {
		if row.Key == "xrayTemplateConfig" && strings.TrimSpace(row.Value) != "" {
			hasXrayTemplate = true
		}
		var cfg map[string]any
		if json.Unmarshal([]byte(row.Value), &cfg) != nil {
			continue
		}
		for _, key := range []string{"outbounds", "endpoints"} {
			if list, ok := cfg[key].([]any); ok {
				for _, raw := range list {
					if item, ok := raw.(map[string]any); ok {
						if tag, ok := item["tag"].(string); ok && !removed[tag] {
							available[tag] = true
						}
					}
				}
			}
		}
	}
	// The default Xray template supplies the canonical direct connection even
	// when it has not needed a persisted settings row yet.
	var base map[string]any
	_ = json.Unmarshal([]byte(defaultValueMap["xrayTemplateConfig"]), &base)
	if list, ok := base["outbounds"].([]any); ok && !hasXrayTemplate {
		for _, raw := range list {
			if item, ok := raw.(map[string]any); ok {
				if tag, ok := item["tag"].(string); ok {
					available[tag] = true
				}
			}
		}
	}
	for _, row := range settings {
		if strings.TrimSpace(row.Value) == "" {
			continue
		}
		var cfg map[string]any
		if err := json.Unmarshal([]byte(row.Value), &cfg); err != nil {
			return err
		}
		implicitDefault := false
		if rows, ok := cfg["outbounds"].([]any); ok && len(rows) > 0 {
			first, _ := rows[0].(map[string]any)
			tag, _ := first["tag"].(string)
			route, _ := cfg["route"].(map[string]any)
			implicitDefault = removed[tag] && (row.Key == "xrayTemplateConfig" || route["final"] == nil || route["final"] == "")
		}
		definitionsChanged := false
		for _, key := range []string{"outbounds", "endpoints"} {
			if rows, ok := cfg[key].([]any); ok {
				kept := make([]any, 0, len(rows))
				for _, raw := range rows {
					row, _ := raw.(map[string]any)
					tag, _ := row["tag"].(string)
					if removed[tag] {
						definitionsChanged = true
						continue
					}
					kept = append(kept, raw)
				}
				cfg[key] = kept
			}
		}
		referenced := replaceConnectionTags(cfg, removed, replacement, "") || implicitDefault
		if !referenced && !definitionsChanged {
			continue
		}
		if referenced && (replacement == "" || removed[replacement] || !available[replacement] && replacement != unavailableSubscriptionTag) {
			return fmt.Errorf("подписка используется в настройках: выберите существующее подключение для замены")
		}
		if referenced && replacement == unavailableSubscriptionTag {
			_, ownsOutbounds := cfg["outbounds"]
			if row.Key == "xrayTemplateConfig" || ownsOutbounds {
				addUnavailableSubscriptionOutbound(cfg, row.Key != "xrayTemplateConfig")
			}
		}
		if implicitDefault {
			if row.Key != "xrayTemplateConfig" {
				route, _ := cfg["route"].(map[string]any)
				if route == nil {
					route = map[string]any{}
					cfg["route"] = route
				}
				route["final"] = replacement
			} else {
				rows, _ := cfg["outbounds"].([]any)
				for i, raw := range rows {
					item, _ := raw.(map[string]any)
					if item["tag"] == replacement {
						cfg["outbounds"] = append([]any{raw}, append(rows[:i], rows[i+1:]...)...)
						break
					}
				}
			}
		}
		data, err := json.Marshal(cfg)
		if err != nil {
			return err
		}
		if err := tx.Model(&model.Setting{}).Where("key = ?", row.Key).Update("value", string(data)).Error; err != nil {
			return err
		}
	}
	if _, err := replaceInfrastructureReferences(tx, removed, replacement); err != nil {
		return err
	}
	return tx.Delete(&model.OutboundSubscription{}, sub.Id).Error
}

func (s *OutboundSubscriptionService) DeleteReplacing(id int, replacement string) error {
	unlock := lockSubscription(id)
	defer unlock()
	subscriptionOrderMu.Lock()
	defer subscriptionOrderMu.Unlock()
	sub, err := s.Get(id)
	if err != nil {
		return err
	}
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		return deleteSubscriptionAndReferences(tx, sub, strings.TrimSpace(replacement))
	})
}
