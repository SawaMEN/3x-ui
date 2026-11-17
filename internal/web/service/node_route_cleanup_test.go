package service

import (
	"encoding/json"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"

	"gorm.io/gorm"
)

func decodeCascadeConfig(t *testing.T, value string) map[string]any {
	t.Helper()
	var cfg map[string]any
	if err := json.Unmarshal([]byte(value), &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestCleanupDeletedXrayReferences(t *testing.T) {
	cfg := decodeCascadeConfig(t, `{
		"routing": {
			"rules": [
				{"type":"field","outboundTag":"node-route-42","inboundTag":["in-a"]},
				{"type":"field","outboundTag":"direct","inboundTag":["in-a","in-b"]},
				{"type":"field","outboundTag":"api","inboundTag":["api"]},
				{"type":"field","outboundTag":"node-route-52","inboundTag":["in-b"]}
			],
			"balancers": [
				{"tag":"node-pool","selector":["node-route-42","node-route-52"]},
				{"tag":"another-pool","selector":["node-route-42"],"fallbackTag":"node-route-42"}
			]
		}
	}`)
	if !cleanupDeletedXrayReferences(cfg, "in-a", "node-route-42") {
		t.Fatal("expected changed config")
	}
	rules := routingRulesFromCfg(cfg)
	if len(rules) != 3 {
		t.Fatalf("rules after deletion = %#v", rules)
	}
	if tags := readInboundTags(rules[0]["inboundTag"]); len(tags) != 1 || tags[0] != "in-b" {
		t.Fatalf("scoped rule broadened or deleted: %#v", rules[0])
	}
	if rules[1]["outboundTag"] != "api" || rules[2]["outboundTag"] != "node-route-52" {
		t.Fatalf("other destinations were lost: %#v", rules)
	}
	balancers := cfg["routing"].(map[string]any)["balancers"].([]any)
	if len(balancers) != 1 {
		t.Fatalf("expected empty balancer to be deleted: %#v", balancers)
	}
	if selectors := readInboundTags(balancers[0].(map[string]any)["selector"]); len(selectors) != 1 || selectors[0] != "node-route-52" {
		t.Fatalf("shared balancer not preserved: %#v", balancers)
	}
}

func TestCleanupDeletedNativeReferences(t *testing.T) {
	cfg := decodeCascadeConfig(t, `{
		"route": {
			"final":"node-route-42",
			"rules": [
				{"comment":"one","inbound":["in-a","in-b"],"outbound":"direct"},
				{"comment":"two","inbound":["in-a"],"outbound":"direct"},
				{"comment":"cascade","inbound":["in-b"],"outbound":"node-route-42"},
				{"comment":"another","inbound":["in-b"],"outbound":"node-route-52"},
				{"comment":"disabled cascade","outbound":"node-route-42","enabled":false}
			]
		}
	}`)
	if !cleanupDeletedNativeReferences(cfg, "in-a", "node-route-42") {
		t.Fatal("expected change")
	}
	route := cfg["route"].(map[string]any)
	if route["final"] != "blocked" {
		t.Fatalf("deleted default was not blocked: %#v", route)
	}
	rules := route["rules"].([]any)
	if len(rules) != 2 {
		t.Fatalf("unexpected remaining rules: %#v", rules)
	}
	one := rules[0].(map[string]any)
	if one["comment"] != "one" || len(readInboundTags(one["inbound"])) != 1 ||
		readInboundTags(one["inbound"])[0] != "in-b" {
		t.Fatalf("other scoped inbounds were lost: %#v", one)
	}
	if rules[1].(map[string]any)["outbound"] != "node-route-52" {
		t.Fatalf("unrelated cascade was deleted: %#v", rules[1])
	}
}

func TestCleanupDeletedInboundReferencesTx(t *testing.T) {
	setupBulkDB(t)
	db := database.GetDB()
	seedXrayTemplate(t, `{"routing":{"rules":[{"outboundTag":"node-route-42"},{"outboundTag":"direct"}]}}`)
	if err := saveSettingIn(db, "singBoxConfigTemplate", `{"route":{"rules":[{"outbound":"node-route-42"},{"outbound":"direct"}],"final":"node-route-42"}}`); err != nil {
		t.Fatal(err)
	}
	if err := saveSettingIn(db, "singBoxUpstreamConfigTemplate", `{"route":{"rules":[{"outbound":"node-route-42"},{"outbound":"direct"}]}}`); err != nil {
		t.Fatal(err)
	}
	if err := saveSettingIn(db, "panelOutbound", "node-route-42"); err != nil {
		t.Fatal(err)
	}
	nodeID := 1
	inbound := &model.Inbound{Id: 42, Tag: "remote", NodeID: &nodeID}
	err := db.Transaction(func(tx *gorm.DB) error {
		changed, err := cleanupDeletedInboundReferencesTx(tx, inbound)
		if err != nil || !changed {
			t.Errorf("cleanup changed=%v err=%v", changed, err)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"xrayTemplateConfig", "singBoxConfigTemplate", "singBoxUpstreamConfigTemplate"} {
		var entry model.Setting
		if err := db.Where("key = ?", key).First(&entry).Error; err != nil {
			t.Fatal(err)
		}
		if !json.Valid([]byte(entry.Value)) || !containsText(entry.Value, "direct") || containsText(entry.Value, "node-route-42") {
			t.Fatalf("dangling reference in %s: %s", key, entry.Value)
		}
	}
	var panel model.Setting
	if err := db.Where("key = ?", "panelOutbound").First(&panel).Error; err != nil {
		t.Fatal(err)
	}
	if panel.Value != "blocked" {
		t.Fatalf("panel destination not fail-closed: %s", panel.Value)
	}
}

func containsText(s, fragment string) bool {
	for i := 0; i+len(fragment) <= len(s); i++ {
		if s[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
