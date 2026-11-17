package service

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

// Include disabled rules, selectors, detours and final routes. A connection
// must not disappear while any saved configuration can still refer to it.
func containsNodeRouteReference(value any, tag string) bool {
	switch value := value.(type) {
	case string:
		return value == tag
	case []any:
		for _, item := range value {
			if containsNodeRouteReference(item, tag) {
				return true
			}
		}
	case map[string]any:
		for _, item := range value {
			if containsNodeRouteReference(item, tag) {
				return true
			}
		}
	}
	return false
}

func nodeRouteReferenced(tx *gorm.DB, tag string) (bool, error) {
	for _, key := range []string{"xrayTemplateConfig", "singBoxConfigTemplate", "singBoxUpstreamConfigTemplate"} {
		var setting model.Setting
		err := tx.Where("key = ?", key).First(&setting).Error
		if errors.Is(err, gorm.ErrRecordNotFound) || err == nil && setting.Value == "" {
			continue
		}
		if err != nil {
			return false, err
		}
		raw := setting.Value
		if key == "xrayTemplateConfig" {
			raw = UnwrapXrayTemplateConfig(raw)
		}
		var cfg any
		if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
			return false, fmt.Errorf("cannot check routing references in %s: %w", key, err)
		}
		if containsNodeRouteReference(cfg, tag) {
			return true, nil
		}
	}
	var count int64
	if err := tx.Model(&model.Setting{}).Where("key = ? AND value = ?", "panelOutbound", tag).Count(&count).Error; err != nil || count > 0 {
		return count > 0, err
	}
	if err := tx.Model(&model.Node{}).Where("outbound_tag = ?", tag).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// DeleteNodeRoute removes only the dedicated credential, never the node or its
// inbound. Ordinary client APIs cannot operate on this identity.
func DeleteNodeRoute(userID, inboundID int) (bool, bool, error) {
	nodeRouteProvisionMu.Lock()
	defer nodeRouteProvisionMu.Unlock()
	db := database.GetDB()
	var inbound model.Inbound
	if err := db.First(&inbound, inboundID).Error; err != nil {
		return false, false, err
	}
	if inbound.UserId != userID || inbound.NodeID == nil {
		return false, false, fmt.Errorf("inbound is not available to this user")
	}
	used, err := nodeRouteReferenced(db, nodeRouteTag(inboundID))
	if err != nil {
		return false, false, err
	}
	if used {
		return false, false, fmt.Errorf("сначала удалите ссылки на это подключение из маршрутизации и сохраните настройки")
	}
	var client model.ClientRecord
	err = db.Where("email = ?", nodeRouteClientEmail(inboundID)).First(&client).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	needRestart, err := (&ClientService{}).Delete(&InboundService{}, client.Id, false)
	if err != nil {
		return false, needRestart, err
	}
	var node model.Node
	if err := db.First(&node, *inbound.NodeID).Error; err != nil {
		return true, needRestart, nil
	}
	if !node.Enable || node.Status != "online" {
		return true, needRestart, nil
	}
	if err := syncNodeRouteInbound(&inbound, &node); err != nil {
		logger.Warningf("Node route removal pending node_id=%d inbound_id=%d error=%s", node.Id, inboundID, logger.ErrorSummary(err))
		return true, needRestart, nil
	}
	return false, needRestart, nil
}
