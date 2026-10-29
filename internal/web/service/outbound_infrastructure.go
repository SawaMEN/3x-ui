package service

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

type infrastructureUndo struct {
	panel       string
	nodeIDs     []int
	previous    string
	replacement string
}

// The panel's egress and remote-node transport are connection references too.
func replaceInfrastructureReferences(tx *gorm.DB, removed map[string]bool, replacement string) (infrastructureUndo, error) {
	undo := infrastructureUndo{replacement: replacement}
	var panel model.Setting
	if err := tx.Where("key = ?", "panelOutbound").Find(&panel).Error; err != nil {
		return undo, err
	}
	if removed[panel.Value] {
		if replacement == "" {
			return undo, fmt.Errorf("подключение используется панелью: выберите замену")
		}
		undo.panel = panel.Value
		if err := tx.Model(&model.Setting{}).Where("key = ?", "panelOutbound").Update("value", replacement).Error; err != nil {
			return undo, err
		}
	}
	var nodes []model.Node
	if err := tx.Select("id", "outbound_tag").Find(&nodes).Error; err != nil {
		return undo, err
	}
	for _, node := range nodes {
		if removed[node.OutboundTag] {
			if replacement == "" {
				return undo, fmt.Errorf("подключение используется узлом %d: выберите замену", node.Id)
			}
			// A single manual deletion has one old tag; subscription deletion can have many,
			// and relies on its encompassing DB transaction instead of this rollback record.
			undo.previous = node.OutboundTag
			undo.nodeIDs = append(undo.nodeIDs, node.Id)
			if err := tx.Model(&model.Node{}).Where("id = ?", node.Id).Update("outbound_tag", replacement).Error; err != nil {
				return undo, err
			}
		}
	}
	return undo, nil
}

func (u infrastructureUndo) restore(tx *gorm.DB) error {
	if u.panel != "" {
		if err := tx.Model(&model.Setting{}).Where("key = ? AND value = ?", "panelOutbound", u.replacement).Update("value", u.panel).Error; err != nil {
			return err
		}
	}
	if len(u.nodeIDs) > 0 {
		return tx.Model(&model.Node{}).Where("id IN ? AND outbound_tag = ?", u.nodeIDs, u.replacement).Update("outbound_tag", u.previous).Error
	}
	return nil
}

func infrastructureUsesBlockedSubscription() (bool, error) {
	var count int64
	if err := database.GetDB().Model(&model.Setting{}).Where("key = ? AND value = ?", "panelOutbound", unavailableSubscriptionTag).Count(&count).Error; err != nil {
		return false, err
	}
	if count > 0 {
		return true, nil
	}
	if err := database.GetDB().Model(&model.Node{}).Where("outbound_tag = ?", unavailableSubscriptionTag).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
