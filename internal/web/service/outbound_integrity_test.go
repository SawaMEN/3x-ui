package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func seedIntegritySubscription(t *testing.T) *model.OutboundSubscription {
	t.Helper()
	setupBulkDB(t)
	sub := &model.OutboundSubscription{Id: 2, Enabled: true, AutoBalance: true, LastFetchedOutbounds: `[]`, TagPrefix: "sub2-"}
	if err := database.GetDB().Create(sub).Error; err != nil {
		t.Fatal(err)
	}
	return sub
}

func TestManagedAliasesCannotDuplicateBeforeNativeTemplateMerge(t *testing.T) {
	seedIntegritySubscription(t)
	// Reproduce copies left by the old runtime editor in both source formats.
	shared := `{"outbounds":[{"protocol":"freedom","tag":"direct"},{"protocol":"loopback","tag":"sub-auto-2","settings":{"inboundTag":"sub-auto-route-2"}},{"protocol":"loopback","tag":"sub-auto-2","settings":{"inboundTag":"sub-auto-route-2"}}]}`
	native := `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"selector","tag":"sub-auto-2","outbounds":["direct"]},{"type":"selector","tag":"sub-auto-2","outbounds":["direct"]}],"route":{"final":"sub-auto-2"}}`
	settings := &SettingService{}
	if err := settings.setString("xrayTemplateConfig", shared); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetSingBoxConfigTemplate(native); err != nil {
		t.Fatal(err)
	}
	cfg, err := NativeCore(CoreTypeSingBox).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, row := range cfg.Outbounds {
		if row["tag"] == "sub-auto-2" {
			count++
			if row["default"] != "sub-auto-block-2" {
				t.Fatal("stale selector survived", row)
			}
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one managed selector, got %d", count)
	}
	snapshot, err := NativeCore(CoreTypeSingBox).GetEditorConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(snapshot.Config["outbounds"])
	if strings.Contains(string(data), `"sub-auto-2"`) {
		t.Fatal("editor leaked managed definition", string(data))
	}
	if len(snapshot.Warnings) == 0 {
		t.Fatal("legacy repair was not explained")
	}
}

func TestDeletedAliasRecoveryBlocksRoutingAndDNSDetour(t *testing.T) {
	setupBulkDB(t)
	raw := `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"selector","tag":"sub-auto-2","outbounds":["direct"]}],"route":{"final":"sub-auto-2","rules":[{"domain":["test.example"],"outbound":"sub-auto-2"}]},"dns":{"servers":[{"type":"udp","tag":"dns","server":"1.1.1.1","detour":"sub-auto-2"}],"final":"dns"}}`
	if err := (&SettingService{}).SetSingBoxConfigTemplate(raw); err != nil {
		t.Fatal(err)
	}
	snapshot, err := NativeCore(CoreTypeSingBox).GetEditorConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	route := snapshot.Config["route"].(map[string]any)
	if route["final"] != unavailableSubscriptionTag {
		t.Fatal("deleted proxy became direct", route)
	}
	data, _ := json.Marshal(snapshot.Config)
	if strings.Contains(string(data), `"sub-auto-2"`) {
		t.Fatal("orphan reference survived", string(data))
	}
	if len(snapshot.Warnings) == 0 {
		t.Fatal("recovery notification missing")
	}
	if _, err := NativeCore(CoreTypeSingBox).GetConfig(); err != nil {
		t.Fatal("recovered config still invalid", err)
	}
	// Old saved source is left intact until Save; enabling a disabled subscription can recover its route.
	stored, _ := (&SettingService{}).GetSingBoxConfigTemplate()
	if stored != raw {
		t.Fatal("read rewrote source settings")
	}
}

func TestSubscriptionDeletionRemovesFrozenDefinitionsAndBlocksReferences(t *testing.T) {
	sub := seedIntegritySubscription(t)
	raw := `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"block","tag":"blocked"},{"type":"selector","tag":"sub-auto-2","outbounds":["direct"]}],"route":{"final":"sub-auto-2"}}`
	if err := (&SettingService{}).SetSingBoxConfigTemplate(raw); err != nil {
		t.Fatal(err)
	}
	if err := (&OutboundSubscriptionService{}).DeleteReplacing(sub.Id, unavailableSubscriptionTag); err != nil {
		t.Fatal(err)
	}
	stored, _ := (&SettingService{}).GetSingBoxConfigTemplate()
	if strings.Contains(stored, `"sub-auto-2"`) {
		t.Fatal("definition or route was retained", stored)
	}
	if _, err := NativeCore(CoreTypeSingBox).GetConfig(); err != nil {
		t.Fatal("deletion left invalid config", err)
	}
}

func TestUnavailableSubscriptionFallbackCannotBeOverridden(t *testing.T) {
	setupBulkDB(t)
	raw := `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"block","tag":"blocked"},{"type":"direct","tag":"xui-unavailable-subscription"}],"route":{"final":"sub-auto-900"}}`
	if err := (&SettingService{}).SetSingBoxConfigTemplate(raw); err != nil {
		t.Fatal(err)
	}
	cfg, err := NativeCore(CoreTypeSingBox).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range cfg.Outbounds {
		if row["tag"] == unavailableSubscriptionTag && row["type"] != "block" {
			t.Fatal("fallback is not blocking", row)
		}
	}
}

func TestManualDeletionRollsBackPanelNodeAndTemplateOnApplyFailure(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir()) // Deliberately no executable: applying must fail safely.
	settings := &SettingService{}
	old := `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"block","tag":"blocked"},{"type":"socks","tag":"retired","server":"127.0.0.1","server_port":1080}]}`
	if err := settings.SetSingBoxConfigTemplate(old); err != nil {
		t.Fatal(err)
	}
	if err := settings.SetPanelOutbound("retired"); err != nil {
		t.Fatal(err)
	}
	node := &model.Node{Name: "test-node", OutboundTag: "retired"}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	candidate := `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"block","tag":"blocked"}]}`
	if err := NativeCore(CoreTypeSingBox).SaveTemplateWithRemoval(context.Background(), candidate, "retired", "direct"); err == nil {
		t.Fatal("missing binary accepted")
	}
	panel, _ := settings.GetPanelOutbound()
	if panel != "retired" {
		t.Fatal("panel egress was not restored", panel)
	}
	var after model.Node
	if err := database.GetDB().First(&after, node.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.OutboundTag != "retired" {
		t.Fatal("node egress was not restored", after.OutboundTag)
	}
	raw, _ := settings.GetSingBoxConfigTemplate()
	if raw != old {
		t.Fatal("template was not restored", raw)
	}
}

func TestSubscriptionDeletionReplacesPanelAndNodeReferences(t *testing.T) {
	sub := seedIntegritySubscription(t)
	settings := &SettingService{}
	if err := settings.SetPanelOutbound(autoOutboundTag(sub.Id)); err != nil {
		t.Fatal(err)
	}
	node := &model.Node{Name: "test-node", OutboundTag: autoOutboundTag(sub.Id)}
	if err := database.GetDB().Create(node).Error; err != nil {
		t.Fatal(err)
	}
	if err := (&OutboundSubscriptionService{}).DeleteReplacing(sub.Id, unavailableSubscriptionTag); err != nil {
		t.Fatal(err)
	}
	panel, _ := settings.GetPanelOutbound()
	if panel != unavailableSubscriptionTag {
		t.Fatal("panel retained a dead subscription", panel)
	}
	var after model.Node
	if err := database.GetDB().First(&after, node.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.OutboundTag != unavailableSubscriptionTag {
		t.Fatal("node retained dead subscription", after.OutboundTag)
	}
	if _, err := NativeCore(CoreTypeSingBox).GetConfig(); err != nil {
		t.Fatal("infrastructure references have no blocking handler", err)
	}
}

func TestOrphanedImplicitDefaultFailsClosed(t *testing.T) {
	setupBulkDB(t)
	source := `{"outbounds":[{"protocol":"loopback","tag":"sub-auto-9","settings":{"inboundTag":"sub-auto-route-9"}},{"protocol":"freedom","tag":"direct"}]}`
	raw, err := NormalizeSubscriptionTemplate(source, false)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatal(err)
	}
	first := cfg["outbounds"].([]any)[0].(map[string]any)
	if first["tag"] != unavailableSubscriptionTag || first["protocol"] != "blackhole" {
		t.Fatal("lost default became a direct connection", first)
	}
}

func TestGeneratedAutomaticRoutingCopiesDoNotAccumulate(t *testing.T) {
	cfg := map[string]any{"routing": map[string]any{"rules": []any{
		map[string]any{"inboundTag": []any{"sub-auto-route-2"}, "outboundTag": "old-member"},
		map[string]any{"inboundTag": []any{"client"}, "outboundTag": "direct"},
	}}}
	cleanSubscriptionSource(cfg, map[string]bool{}, map[string]bool{})
	rules := cfg["routing"].(map[string]any)["rules"].([]any)
	if len(rules) != 1 || rules[0].(map[string]any)["outboundTag"] != "direct" {
		t.Fatal("managed loopback rule survived or manual rule removed", rules)
	}
}
