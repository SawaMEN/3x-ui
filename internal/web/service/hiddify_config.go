package service

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// HiddifyConfigGenerator builds the full server configuration used by previews,
// candidate validation and runtime publication. It does not write panel rows.
type HiddifyConfigGenerator struct{ StrictProtocolServers model.Protocol }

func (g *HiddifyConfigGenerator) Generate() (*singbox.Config, error) {
	snapshot, err := loadNativeConfigSnapshot()
	if err != nil {
		return nil, fmt.Errorf("sing-box configuration snapshot: %w", err)
	}
	core := *NativeCore(CoreTypeHiddify)
	core.strictProtocolServers = g.StrictProtocolServers
	return core.generateNativeConfig(snapshot)
}

type nativeConfigSnapshot struct {
	inbounds      []*model.Inbound
	clients       map[int][]model.Client
	settings      map[string]string
	subscriptions []*model.OutboundSubscription
	routingLists  []model.RoutingList
	nodes         []*model.Node
	capturedAt    time.Time
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
	return loadNativeConfigSnapshotForTemplate(nil)
}

func loadNativeConfigSnapshotForTemplate(template *string) (*nativeConfigSnapshot, error) {
	snapshot := &nativeConfigSnapshot{settings: map[string]string{}, capturedAt: time.Now()}
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
		if template != nil {
			snapshot.settings["xrayTemplateConfig"] = *template
		}
		listIDs, err := routingListIDs(snapshot.setting("xrayTemplateConfig"))
		if err != nil {
			return err
		}
		nativeListIDs, err := routingListIDs(snapshot.setting("singBoxConfigTemplate"))
		if err != nil {
			return err
		}
		listIDs = append(listIDs, nativeListIDs...)
		if len(listIDs) > 0 {
			if err := tx.Where("id IN ?", listIDs).Find(&snapshot.routingLists).Error; err != nil {
				return err
			}
		}
		if err := tx.Select("id", "enable", "outbound_tag").Order("sort_order ASC, id ASC").Find(&snapshot.nodes).Error; err != nil {
			return err
		}
		return tx.Where("enabled = ?", true).Order("priority ASC, id ASC").Find(&snapshot.subscriptions).Error
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	return snapshot, err
}

// All overlays consume the same snapshot as listeners and routing. In particular,
// an atomic settings update cannot split the filtering policy across generations.
func (s *nativeConfigSnapshot) applyOverlays(cfg *singbox.Config) error {
	if tag := strings.TrimSpace(s.setting("panelOutbound")); tag != "" {
		if !injectSingBoxEgressBridge(cfg, PanelEgressInboundTag, tag, panelEgressBasePort, "panel") {
			return fmt.Errorf("hiddify cannot create panel egress bridge for %q", tag)
		}
	}
	for _, node := range s.nodes {
		if !node.Enable || strings.TrimSpace(node.OutboundTag) == "" {
			continue
		}
		if !injectSingBoxEgressBridge(cfg, NodeEgressInboundTag(node.Id), node.OutboundTag, nodeEgressBasePort+node.Id, fmt.Sprintf("node %d", node.Id)) {
			return fmt.Errorf("hiddify cannot create node %d egress bridge for %q", node.Id, node.OutboundTag)
		}
	}
	values := make(map[string]string, len(defaultValueMap))
	for key, value := range defaultValueMap {
		values[key] = value
	}
	for key, value := range s.settings {
		values[key] = value
	}
	plan, err := adBlockPlanAt(values, s.capturedAt)
	if err != nil {
		return fmt.Errorf("hiddify filtering policy: %w", err)
	}
	if len(plan.Policies) > 0 {
		err = applySingBoxPolicyPlan(cfg, plan)
	} else {
		var domains []string
		if plan.Enabled {
			domains = plan.Base
		}
		err = applySingBoxAdBlockDomains(cfg, domains, plan.Scope)
	}
	if err != nil {
		return err
	}
	var server AdBlockServerSettings
	if err := json.Unmarshal([]byte(s.setting("adBlockServer")), &server); err != nil {
		return err
	}
	return applySingBoxYouTubeServerPlan(cfg, server, youtubeServerPlan(server, plan))
}
