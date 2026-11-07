package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"gorm.io/gorm"
)

// Delete the destination and all of its route references together with the
// inbound row. In particular, node-route-ID is never persisted as an outbound:
// it is generated from the remote inbound ID at runtime.
func cleanupDeletedInboundReferencesTx(tx *gorm.DB, inbound *model.Inbound) (bool, error) {
	if inbound == nil || inbound.Id <= 0 {
		return false, nil
	}
	target := ""
	if inbound.NodeID != nil {
		target = nodeRouteTag(inbound.Id)
	}
	changed := false
	for _, key := range []string{"xrayTemplateConfig", "singBoxConfigTemplate", "singBoxUpstreamConfigTemplate"} {
		var setting model.Setting
		err := tx.Where("key = ?", key).First(&setting).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return false, err
		}
		if strings.TrimSpace(setting.Value) == "" {
			continue
		}
		var cfg map[string]any
		if err := json.Unmarshal([]byte(setting.Value), &cfg); err != nil {
			return false, fmt.Errorf("cleanup deleted inbound: invalid %s: %w", key, err)
		}
		if cfg == nil {
			continue
		}
		var modified bool
		if key == "xrayTemplateConfig" {
			modified = cleanupDeletedXrayReferences(cfg, inbound.Tag, target)
		} else {
			modified = cleanupDeletedNativeReferences(cfg, inbound.Tag, target)
		}
		if !modified {
			continue
		}
		value, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return false, err
		}
		if err := tx.Model(&model.Setting{}).Where("key = ?", key).Update("value", string(value)).Error; err != nil {
			return false, err
		}
		changed = true
	}
	if target != "" {
		// These are infrastructure references, not routing rules. Preserve
		// fail-closed behavior when the destination no longer exists.
		if err := tx.Model(&model.Setting{}).Where("key = ? AND value = ?", "panelOutbound", target).
			Update("value", "blocked").Error; err != nil {
			return false, err
		}
		if err := tx.Model(&model.Node{}).Where("outbound_tag = ?", target).
			Update("outbound_tag", "blocked").Error; err != nil {
			return false, err
		}
	}
	return changed, nil
}

func cleanupDeletedXrayReferences(cfg map[string]any, inboundTag, target string) bool {
	changed := false
	rules := routingRulesFromCfg(cfg)
	if len(rules) > 0 {
		cleaned, removed := removeInboundTagFromRules(rules, inboundTag)
		if removed {
			changed = true
		}
		if target != "" {
			remaining := make([]map[string]any, 0, len(cleaned))
			for _, rule := range cleaned {
				if rule["outboundTag"] == target {
					changed = true
					continue
				}
				remaining = append(remaining, rule)
			}
			cleaned = remaining
		}
		if changed {
			setRoutingRulesInCfg(cfg, cleaned)
		}
	}
	outbounds := outboundsFromCfg(cfg)
	if len(outbounds) > 0 && removeInboundTagFromOutbounds(outbounds, inboundTag) {
		cfg["outbounds"] = outbounds
		changed = true
	}
	if target == "" {
		return changed
	}
	routing, _ := cfg["routing"].(map[string]any)
	if routing == nil {
		return changed
	}
	balancers, _ := routing["balancers"].([]any)
	if len(balancers) == 0 {
		return changed
	}
	kept := make([]any, 0, len(balancers))
	removedTags := map[string]bool{}
	for _, item := range balancers {
		balancer, ok := item.(map[string]any)
		if !ok {
			kept = append(kept, item)
			continue
		}
		selectors := readInboundTags(balancer["selector"])
		if len(selectors) > 0 {
			next := make([]string, 0, len(selectors))
			for _, selector := range selectors {
				if selector != target {
					next = append(next, selector)
				}
			}
			if len(next) != len(selectors) {
				changed = true
				if len(next) == 0 {
					if tag, ok := balancer["tag"].(string); ok {
						removedTags[tag] = true
					}
					continue
				}
				balancer["selector"] = next
			}
		}
		if balancer["fallbackTag"] == target {
			delete(balancer, "fallbackTag")
			changed = true
		}
		kept = append(kept, balancer)
	}
	if changed {
		routing["balancers"] = kept
	}
	if len(removedTags) > 0 {
		filtered := make([]map[string]any, 0, len(routingRulesFromCfg(cfg)))
		for _, rule := range routingRulesFromCfg(cfg) {
			if tag, _ := rule["balancerTag"].(string); removedTags[tag] {
				continue
			}
			filtered = append(filtered, rule)
		}
		setRoutingRulesInCfg(cfg, filtered)
	}
	return changed
}

// A native route may scope several inbounds. Keep its other scopes; never
// convert a scoped rule into a global catch-all after deleting its last inbound.
func cleanupNativeRule(rule map[string]any, inboundTag, target string) (bool, bool) {
	if target != "" && rule["outbound"] == target {
		return false, true
	}
	tags := readInboundTags(rule["inbound"])
	if inboundTag != "" && len(tags) > 0 {
		next := make([]string, 0, len(tags))
		for _, tag := range tags {
			if tag != inboundTag {
				next = append(next, tag)
			}
		}
		if len(next) != len(tags) {
			if len(next) == 0 {
				return false, true
			}
			rule["inbound"] = next
			return true, false
		}
	}
	return false, false
}

func cleanupDeletedNativeReferences(cfg map[string]any, inboundTag, target string) bool {
	routing, _ := cfg["route"].(map[string]any)
	if routing == nil {
		return false
	}
	changed := false
	if rules, ok := routing["rules"].([]any); ok {
		kept := make([]any, 0, len(rules))
		for _, raw := range rules {
			rule, ok := raw.(map[string]any)
			if !ok {
				kept = append(kept, raw)
				continue
			}
			modified, remove := cleanupNativeRule(rule, inboundTag, target)
			if modified || remove {
				changed = true
			}
			if !remove {
				kept = append(kept, rule)
			}
		}
		if changed {
			routing["rules"] = kept
		}
	}
	if target != "" && routing["final"] == target {
		// The generated native core always supplies a blocked outbound.
		routing["final"] = "blocked"
		changed = true
	}
	return changed
}

// A node-route service client belongs to exactly one remote inbound. Remove
// only the dedicated record after detaching that inbound, and never touch a
// client that is still linked to another inbound.
func cleanupManagedNodeRouteClientTx(tx *gorm.DB, inbound *model.Inbound) error {
	if inbound == nil || inbound.NodeID == nil {
		return nil
	}
	email := nodeRouteClientEmail(inbound.Id)
	var client model.ClientRecord
	err := tx.Where("email = ?", email).First(&client).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	var linked int64
	if err := tx.Model(&model.ClientInbound{}).Where("client_id = ?", client.Id).Count(&linked).Error; err != nil {
		return err
	}
	if linked > 0 {
		return nil
	}
	if err := tx.Where("client_id = ?", client.Id).Delete(&model.ClientExternalLink{}).Error; err != nil {
		return err
	}
	if err := clearClientHwidsBySubIDTx(tx, client.SubID); err != nil {
		return err
	}
	if err := tx.Where("email = ?", email).Delete(&coremodel.ClientTraffic{}).Error; err != nil {
		return err
	}
	if err := clearGlobalTraffic(tx, email); err != nil {
		return err
	}
	if err := tx.Where("client_email = ?", email).Delete(&model.InboundClientIps{}).Error; err != nil {
		return err
	}
	if err := tx.Where("email = ?", email).Delete(&model.NodeClientTraffic{}).Error; err != nil {
		return err
	}
	return tx.Delete(&client).Error
}
