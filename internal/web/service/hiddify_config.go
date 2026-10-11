package service

import (
	"database/sql"
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"gorm.io/gorm"
)

// HiddifyConfigGenerator builds the full server configuration used by previews,
// candidate validation and runtime publication. It does not write panel rows.
type HiddifyConfigGenerator struct{}

func (g *HiddifyConfigGenerator) Generate() (*singbox.Config, error) {
	snapshot, err := loadNativeConfigSnapshot()
	if err != nil {
		return nil, fmt.Errorf("hiddify configuration snapshot: %w", err)
	}
	return NativeCore(CoreTypeHiddify).generateNativeConfig(snapshot)
}

type nativeConfigSnapshot struct {
	inbounds      []*model.Inbound
	clients       map[int][]model.Client
	settings      map[string]string
	subscriptions []*model.OutboundSubscription
}

func (s *nativeConfigSnapshot) setting(key string) string {
	if value, ok := s.settings[key]; ok {
		return value
	}
	return defaultValueMap[key]
}

func (s *nativeConfigSnapshot) subscription(id int) (*model.OutboundSubscription, error) {
	for _, sub := range s.subscriptions {
		if sub.Id == id {
			return sub, nil
		}
	}
	return nil, fmt.Errorf("outbound subscription %d is not enabled in this configuration snapshot", id)
}

func nativeConfigSetting(snapshot *nativeConfigSnapshot, key string, fallback func() (string, error)) (string, error) {
	if snapshot != nil {
		return snapshot.setting(key), nil
	}
	return fallback()
}

// Read inbounds, quota state, canonical linked users, fallbacks and templates
// together. Do not invoke UI readers which reconcile traffic rows as a side effect.
func loadNativeConfigSnapshot() (*nativeConfigSnapshot, error) {
	snapshot := &nativeConfigSnapshot{settings: map[string]string{}}
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := tx.Preload("ClientStats").Order("id ASC").Find(&snapshot.inbounds).Error; err != nil {
			return err
		}
		ids := make([]int, 0, len(snapshot.inbounds))
		for i, inbound := range snapshot.inbounds {
			if !inbound.Enable || inbound.NodeID != nil || isLocalSidecarInbound(inbound.Protocol) {
				continue
			}
			ids = append(ids, inbound.Id)
			built, err := singBoxInboundService.buildInboundForNodePush(tx, inbound)
			if err != nil {
				return fmt.Errorf("inbound %q: %w", inbound.Tag, err)
			}
			snapshot.inbounds[i] = built
		}
		var err error
		snapshot.clients, err = singBoxInboundService.clientService.ListForInbounds(tx, ids)
		if err != nil {
			return err
		}
		var settings []model.Setting
		if err := tx.Find(&settings).Error; err != nil {
			return err
		}
		for _, setting := range settings {
			snapshot.settings[setting.Key] = setting.Value
		}
		return tx.Where("enabled = ?", true).Order("priority ASC, id ASC").Find(&snapshot.subscriptions).Error
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return snapshot, err
}
