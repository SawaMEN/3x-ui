package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	xraygeodata "github.com/xtls/xray-core/common/geodata"
	"google.golang.org/protobuf/proto"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

func TestRoutingCatalogDownloadCannotInheritPrivateNetworkOptIn(t *testing.T) {
	ctx := netsafe.ContextWithAllowPrivate(context.Background(), true)
	for _, source := range []string{"https://127.0.0.1/list?token=secret", "https://[::1]/list"} {
		_, err := downloadRoutingList(ctx, source)
		if !errors.Is(err, netsafe.ErrPrivateAddressBlocked) {
			t.Fatalf("private source not blocked: %v", err)
		}
		if strings.Contains(err.Error(), "token=secret") {
			t.Fatal("source token leaked into error")
		}
	}
}

func TestRoutingCatalogBuiltinUsesInstalledGeoSite(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XRAY_LOCATION_ASSET", dir)
	data, err := proto.Marshal(&xraygeodata.GeoSiteList{Entry: []*xraygeodata.GeoSite{
		{Code: "youtube", Domain: []*xraygeodata.Domain{{Type: xraygeodata.Domain_Domain, Value: "youtube.com"}}},
		{Code: "empty", Domain: []*xraygeodata.Domain{}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "geosite.dat"), data, 0600); err != nil {
		t.Fatal(err)
	}
	var youtube, discord *RoutingCatalogCategory
	for _, category := range (&RoutingCatalogService{}).Categories() {
		if category.ID == "youtube" {
			copy := category
			youtube = &copy
		}
		if category.ID == "discord" {
			copy := category
			discord = &copy
		}
	}
	if youtube == nil || !youtube.Available || discord == nil || discord.Available {
		t.Fatal("catalog did not reflect installed categories")
	}
	template := `{"route":{"rules":[{"routingListCategory":"youtube","action":"route","outbound":"vpn","inbound":["client"]}]}}`
	generated, err := materializeNativeRoutingLists(template, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(generated, `"domain_suffix":["youtube.com"]`) || strings.Contains(generated, "routingListCategory") || !strings.Contains(generated, `"inbound":["client"]`) {
		t.Fatalf("category was not translated with scope preserved: %s", generated)
	}
	for _, code := range []string{"missing", "empty"} {
		if _, err := materializeNativeRoutingLists(strings.Replace(template, "youtube", code, 1), nil); err == nil {
			t.Fatal("missing/empty category became a catch-all")
		}
	}
}

func TestRoutingCatalogMaterializationPreservesOrderAndMatchesEitherKind(t *testing.T) {
	template := `{"routing":{"rules":[{"type":"field","inboundTag":["api"],"outboundTag":"api"},{"type":"field","routingListId":1,"outboundTag":"vpn","user":["client"],"ruleTag":"catalog-list-1"},{"type":"field","outboundTag":"direct"}]}}`
	sources := []model.RoutingList{{Id: 1, Entries: `{"domains":["domain:example.com"],"ips":["203.0.113.0/24"]}`}}
	got, err := materializeRoutingLists(template, sources)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatal(err)
	}
	rules := routingRulesFromCfg(cfg)
	if len(rules) != 4 || rules[0]["outboundTag"] != "api" || rules[3]["outboundTag"] != "direct" {
		t.Fatalf("wrong order: %v", rules)
	}
	if _, ok := rules[1]["ip"]; ok {
		t.Fatal("domains and IPs must be OR, not AND")
	}
	if _, ok := rules[2]["domain"]; ok {
		t.Fatal("IP rule must not require a domain")
	}
	for _, rule := range rules[1:3] {
		if rule["routingListId"] != nil || rule["outboundTag"] != "vpn" || len(rule["user"].([]any)) != 1 {
			t.Fatalf("lost target/scope or leaked metadata: %v", rule)
		}
	}
	if _, err := singbox.TranslateXrayRouting(cfg["routing"].(map[string]any)); err != nil {
		t.Fatalf("native routing rejected: %v", err)
	}
	if ruleHasNonInboundMatchers(map[string]any{"routingListId": 1}) == false {
		t.Fatal("catalog must count as a matcher during inbound reference cleanup")
	}
}

func TestRoutingCatalogNativeSnapshotRuntimeAndEditorRoundTrip(t *testing.T) {
	setupSettingTestDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	row := model.RoutingList{Name: "custom", Entries: `{"domains":["domain:example.com"],"ips":["203.0.113.0/24"]}`}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	nativeTemplate := `{"route":{"final":"direct","rules":[{"action":"sniff"},{"routingListId":1,"enabled":true,"comment":"custom","action":"route","outbound":"direct","network":["tcp"]}]}}`
	if err := (&SettingService{}).SetSingBoxConfigTemplate(nativeTemplate); err != nil {
		t.Fatal(err)
	}
	snapshot, err := loadNativeConfigSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.routingLists) != 1 {
		t.Fatal("native list references were not captured")
	}
	cfg, err := NativeCore(CoreTypeHiddify).generateNativeConfig(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "domain_suffix") || !strings.Contains(string(raw), "203.0.113.0/24") || strings.Contains(string(raw), "routingListId") || strings.Contains(string(raw), `"comment":`) {
		t.Fatalf("invalid native runtime policy: %s", raw)
	}
	// The editor reads the expanded runtime file but must return source
	// references, not freeze a list when unrelated settings are saved.
	native := NativeCore(CoreTypeHiddify)
	if err := os.MkdirAll(filepath.Dir(native.ProcessConfigPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(native.ProcessConfigPath(), raw, 0600); err != nil {
		t.Fatal(err)
	}
	editor, err := native.GetEditorConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	view, err := json.Marshal(editor.Config)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(view), `"routingListId":1`) || strings.Contains(string(view), "example.com") {
		t.Fatalf("editor lost managed reference: %s", view)
	}
	if err := (&RoutingCatalogService{}).Delete(row.Id); err == nil {
		t.Fatal("deleted a source referenced by native config")
	}
	if err := database.GetDB().Model(&row).Update("entries", `{"domains":["domain:updated.example.com"],"ips":[]}`).Error; err != nil {
		t.Fatal(err)
	}
	// A captured snapshot keeps its original policy even after cache updates.
	again, err := NativeCore(CoreTypeHiddify).generateNativeConfig(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	previous, _ := again.Marshal()
	if strings.Contains(string(previous), "updated.example.com") {
		t.Fatal("generation read outside its snapshot")
	}
	fresh, err := NativeCore(CoreTypeHiddify).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	updated, _ := fresh.Marshal()
	if !strings.Contains(string(updated), "updated.example.com") {
		t.Fatal("runtime did not consume the new cached source")
	}
}

func TestRoutingCatalogNativeMissingSourceNeverMatchesAll(t *testing.T) {
	for _, template := range []string{
		`{"route":{"rules":[{"routingListId":1,"outbound":"vpn"}]}}`,
		`{"route":{"rules":[{"routingListCategory":"","outbound":"vpn"}]}}`,
		`{"route":{"rules":[{"routingListId":1,"domain":["example.com"],"outbound":"vpn"}]}}`,
	} {
		if _, err := materializeNativeRoutingLists(template, nil); err == nil {
			t.Fatal("invalid native catalog policy accepted")
		}
	}
}

func TestRoutingCatalogNeverTurnsMissingOrEmptySourceIntoCatchAll(t *testing.T) {
	template := `{"routing":{"rules":[{"type":"field","routingListId":1,"outboundTag":"vpn"}]}}`
	for _, sources := range [][]model.RoutingList{nil, {{Id: 1, Entries: `{}`}}, {{Id: 1, Entries: `invalid`}}, {{Id: 1, Entries: `{"domains":[""]}`}}} {
		if _, err := materializeRoutingLists(template, sources); err == nil {
			t.Fatal("invalid cache accepted")
		}
	}
	if _, err := materializeRoutingLists(strings.Replace(template, `"routingListId":1`, `"routingListId":1.5`, 1), nil); err == nil {
		t.Fatal("noninteger ID accepted")
	}
	disabled := strings.Replace(template, `"routingListId":1`, `"enabled":false,"routingListId":1`, 1)
	got, err := materializeRoutingLists(disabled, nil)
	if err != nil || strings.Contains(got, "outboundTag") {
		t.Fatalf("disabled rule was not removed: %s, %v", got, err)
	}
}

func TestRoutingCatalogFailedRefreshRetainsCacheAndDeletionIsGuarded(t *testing.T) {
	setupSettingTestDB(t)
	row := model.RoutingList{Name: "custom", URL: "https://127.0.0.1/list", UpdateInterval: 3600, Entries: `{"domains":["domain:example.com"],"ips":[]}`, DomainCount: 1, LastUpdated: 123}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	svc := &RoutingCatalogService{}
	if changed, err := svc.Refresh(context.Background(), row.Id); err == nil || changed {
		t.Fatalf("private request accepted: %v %v", changed, err)
	}
	var stored model.RoutingList
	if err := database.GetDB().First(&stored, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Entries != row.Entries || stored.LastUpdated != 123 || stored.LastError == "" || stored.LastAttempt == 0 {
		t.Fatalf("failed refresh destroyed cache: %#v", stored)
	}
	template := `{"routing":{"rules":[{"type":"field","routingListId":1,"outboundTag":"direct"}]}}`
	if err := (&SettingService{}).saveSetting("xrayTemplateConfig", template); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(row.Id); err == nil {
		t.Fatal("deleted an active routing source")
	}
	if err := (&SettingService{}).saveSetting("xrayTemplateConfig", `{"routing":{"rules":[]}}`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(row.Id); err != nil {
		t.Fatal(err)
	}
}

func TestRoutingCatalogCandidateSnapshotIncludesNewReferences(t *testing.T) {
	setupSettingTestDB(t)
	row := model.RoutingList{Name: "custom", Entries: `{"domains":["domain:example.com"],"ips":[]}`}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if err := (&SettingService{}).saveSetting("xrayTemplateConfig", `{"routing":{"rules":[]}}`); err != nil {
		t.Fatal(err)
	}
	candidate := `{"outbounds":[{"protocol":"freedom","tag":"direct","settings":{}}],"routing":{"rules":[{"type":"field","routingListId":1,"outboundTag":"direct"}]}}`
	snapshot, err := loadNativeConfigSnapshotForTemplate(&candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.routingLists) != 1 || snapshot.routingLists[0].Id != row.Id {
		t.Fatal("candidate used sources from the old template")
	}
	if _, err := materializeRoutingLists(snapshot.setting("xrayTemplateConfig"), snapshot.routingLists); err != nil {
		t.Fatal(err)
	}
	generated, err := NativeCore(CoreTypeHiddify).generateNativeConfig(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := generated.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "example.com") || strings.Contains(string(raw), "routingListId") {
		t.Fatalf("runtime policy missing or metadata leaked: %s", raw)
	}
	stored, err := (&SettingService{}).GetXrayConfigTemplate()
	if err != nil || stored != `{"routing":{"rules":[]}}` {
		t.Fatal("candidate validation rewrote saved template")
	}
}
