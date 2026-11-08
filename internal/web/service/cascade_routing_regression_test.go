package service

import (
	"fmt"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestSubscriptionMergePreservesDefaultOrder(t *testing.T) {
	for _, tags := range [][]string{{"sub-auto-1", "sub-auto-block-1", "exit", "direct"}, {"direct", "exit"}} {
		var items []map[string]any
		managed := map[string]bool{}
		for _, tag := range tags {
			items = append(items, map[string]any{"tag": tag})
			managed[tag] = tag != "direct"
		}
		merged := mergeSubscriptionConnections(items, selectSubscriptionConnections(items, managed), managed)
		for i, tag := range tags {
			if merged[i]["tag"] != tag {
				t.Fatalf("default/order changed: %v", merged)
			}
		}
	}
}

func TestTUICCascadeRetainsDomainSniffing(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	cert, key := writeTestCertificate(t)
	ib := seedHiddifyConfigInbound(t, model.TUIC, fmt.Sprintf(`{"server":{"certificate":%q,"private_key":%q,"alpn":["h3"],"congestion_control":"bbr","authentication_timeout":5}}`, cert, key), `{}`)
	cfg, err := (&HiddifyConfigGenerator{}).Generate()
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range singBoxRouteRules(cfg.Route) {
		rule, ok := raw.(map[string]any)
		if ok && rule["action"] == "sniff" {
			for _, tag := range readInboundTags(rule["inbound"]) {
				if tag == ib.Tag {
					return
				}
			}
		}
	}
	t.Fatal("TUIC bypasses domain/protocol routing because its sniff rule is missing")
}

func TestSubscriptionMergeRefreshesCredentialsWithoutMovingManualRules(t *testing.T) {
	template := []map[string]any{
		{"tag": "exit", "password": "stale"},
		{"tag": "direct", "type": "direct"},
		{"tag": "exit", "password": "duplicate"},
	}
	generated := []map[string]any{{"tag": "exit", "password": "current"}, {"tag": "new-hop"}}
	tags := map[string]bool{"exit": true, "new-hop": true, "endpoint": true}
	merged := mergeSubscriptionConnections(template, generated, tags)
	if len(merged) != 3 || merged[0]["password"] != "current" || merged[1]["tag"] != "direct" || merged[2]["tag"] != "new-hop" {
		t.Fatalf("refresh corrupted default, credentials or order: %v", merged)
	}
	if template[0]["password"] != "stale" {
		t.Fatal("editor source mutated")
	}
}
