package service

import (
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/util/link"
)

func TestSubscriptionNativeJSONRetagsCascadeAndGroups(t *testing.T) {
	body := `{"outbounds":[{"type":"ssh","tag":"hop","server":"example.com","user":"root"},{"type":"anytls","tag":"exit","server":"example.net","server_port":443,"password":"test","detour":"hop","headers":{"detour":"hop"}},{"type":"selector","tag":"group","outbounds":["exit"],"default":"exit"}],"endpoints":[{"type":"masque-client","tag":"tunnel","detour":"group","server":"example.org"}]}`
	for _, encoded := range []bool{false, true} {
		input := body
		if encoded {
			input = base64.RawURLEncoding.EncodeToString([]byte(body))
		}
		rows, ids, err := parseOutboundSubscription([]byte(input))
		if err != nil || len(rows) != 4 {
			t.Fatalf("rows=%v err=%v", rows, err)
		}
		assigned := assignStableTags(rows, ids, nil, nil, 1, "test-")
		settings := rows[1]["settings"].(map[string]any)
		if settings["detour"] != assigned[0] || settings["headers"].(map[string]any)["detour"] != "hop" {
			t.Fatalf("cascade/header retag: %v", settings)
		}
		group := rows[2]["settings"].(map[string]any)
		if group["outbounds"].([]any)[0] != assigned[1] || group["default"] != assigned[1] {
			t.Fatalf("group retag: %v", group)
		}
		if rows[3]["settings"].(map[string]any)["detour"] != assigned[2] {
			t.Fatalf("endpoint detour: %v", rows[3])
		}
		previous := map[string]string{}
		for i, id := range ids {
			previous[id] = assigned[i]
		}
		fresh, freshIDs, err := parseOutboundSubscription([]byte(body))
		if err != nil {
			t.Fatal(err)
		}
		fresh[0], fresh[1] = fresh[1], fresh[0]
		freshIDs[0], freshIDs[1] = freshIDs[1], freshIDs[0]
		updated := assignStableTags(fresh, freshIDs, previous, nil, 1, "test-")
		if updated[0] != assigned[1] || fresh[0]["settings"].(map[string]any)["detour"] != assigned[0] {
			t.Fatalf("refresh broke cascade: %v", fresh)
		}
	}
}

func TestSubscriptionNativeURIAlongsideLegacyLinks(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	body := "vless://11111111-1111-4111-8111-111111111111@example.org:443?security=tls#one\nanytls://secret@example.com:443#two\ntuic://11111111-1111-4111-8111-111111111111:secret@example.net:443#three\nssh://root:secret@example.org:22#four"
	rows, _, err := parseOutboundSubscription([]byte(body))
	if err != nil || len(rows) != 4 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}
	for i, want := range []string{"vless", "singbox:anytls", "singbox:tuic", "singbox:ssh"} {
		if rows[i]["protocol"] != want {
			t.Fatalf("protocol[%d]=%v", i, rows[i]["protocol"])
		}
	}
}

func TestSubscriptionRejectsAmbiguousJSONTags(t *testing.T) {
	_, _, err := parseOutboundSubscription([]byte(`[{"type":"ssh","tag":"same"},{"type":"anytls","tag":"same","detour":"same"}]`))
	if err == nil {
		t.Fatal("duplicate source tags accepted")
	}
	_, _, err = parseOutboundSubscription([]byte(`{"endpoints":[{"type":"masque-server","tag":"listener"}]}`))
	if err == nil {
		t.Fatal("subscription imported a server listener")
	}
}

func TestSubscriptionCascadePrunesBrokenHopAndSelectsOnlyRoots(t *testing.T) {
	hop := map[string]any{"tag": "hop", "protocol": "singbox:shadowtls"}
	exit := map[string]any{"tag": "exit", "protocol": "singbox:anytls", "settings": map[string]any{"detour": "hop"}}
	group := map[string]any{"tag": "group", "protocol": "singbox:selector", "settings": map[string]any{"outbounds": []any{"exit"}}}
	members := []any{hop, exit, group, map[string]any{"tag": "direct", "protocol": "singbox:direct"}}
	if roots := subscriptionCascadeCandidates(members); len(roots) != 1 || roots[0].(map[string]any)["tag"] != "group" {
		t.Fatalf("roots=%v", roots)
	}
	if kept, issues := pruneSubscriptionDependencies(members, []any{group, exit}); len(kept) != 0 || len(issues) != 2 {
		t.Fatalf("kept=%v issues=%v", kept, issues)
	}
	sub := &model.OutboundSubscription{Id: 1}
	if target := effectiveAutoTarget(sub, members); target != "group" {
		t.Fatalf("initial selection bypassed cascade: %s", target)
	}
	sub.SelectedTag = "hop"
	if target := effectiveAutoTarget(sub, members); target != autoBlockedTag(1) {
		t.Fatalf("stale inner hop selected: %s", target)
	}
}

func TestSubscriptionSurvivesNativeTemplateOverride(t *testing.T) {
	setupBulkDB(t)
	body := `{"outbounds":[{"type":"ssh","tag":"hop","server":"example.com","user":"root"},{"type":"anytls","tag":"exit","server":"example.net","server_port":443,"password":"test","tls":{"enabled":true},"detour":"hop"}],"endpoints":[{"type":"masque-client","tag":"tunnel","server":"example.org","detour":"exit"}]}`
	rows, ids, err := parseOutboundSubscription([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	assigned := assignStableTags(rows, ids, nil, nil, 1, "test-")
	stored, _ := json.Marshal(rows)
	sub := &model.OutboundSubscription{Enabled: true, AutoBalance: true, LastFetchedOutbounds: string(stored)}
	if err := database.GetDB().Create(sub).Error; err != nil {
		t.Fatal(err)
	}
	patch := `{"outbounds":[{"type":"direct","tag":"direct"},{"type":"block","tag":"blocked"}],"endpoints":[]}`
	if err := singBoxSettingService.SetSingBoxConfigTemplate(patch); err != nil {
		t.Fatal(err)
	}
	cfg, err := NativeCore(CoreTypeHiddify).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	tags := map[string]bool{}
	for _, row := range cfg.Outbounds {
		tags[row["tag"].(string)] = true
	}
	for _, row := range cfg.Endpoints {
		tags[row["tag"].(string)] = true
	}
	for _, tag := range append(assigned, autoOutboundTag(sub.Id)) {
		if !tags[tag] {
			t.Fatalf("subscription tag %s lost from native config", tag)
		}
	}
	if len(cfg.Endpoints) != 1 || cfg.Endpoints[0]["type"] != "masque-client" {
		t.Fatalf("wrong endpoint placement: %v", cfg.Endpoints)
	}
}

func TestStableTagsRetagLegacyCascadeWithoutRewritingCredentials(t *testing.T) {
	rows := []link.Outbound{
		{"tag": "hop", "protocol": "socks"},
		{"tag": "exit", "protocol": "vless", "proxySettings": map[string]any{"tag": "hop"}, "streamSettings": map[string]any{"sockopt": map[string]any{"dialerProxy": "hop"}}, "settings": map[string]any{"password": "hop"}},
	}
	assigned := assignStableTags(rows, nil, nil, nil, 1, "test-")
	if rows[1]["proxySettings"].(map[string]any)["tag"] != assigned[0] || rows[1]["streamSettings"].(map[string]any)["sockopt"].(map[string]any)["dialerProxy"] != assigned[0] {
		t.Fatalf("legacy cascade not retagged: %v", rows)
	}
	if rows[1]["settings"].(map[string]any)["password"] != "hop" {
		t.Fatal("credential rewritten")
	}
}
