package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestSingBoxEditorRoundTripsDoNotAccumulateRuntimeRules(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	seedHiddifyConfigInbound(t, model.VLESS, `{"decryption":"none"}`, `{"network":"tcp","security":"none"}`)
	svc := &SingBoxService{}
	cfg, err := svc.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	var sniff any
	for _, raw := range singBoxRouteRules(cfg.Route) {
		if rule, ok := raw.(map[string]any); ok && rule["action"] == "sniff" {
			sniff = rule
		}
	}
	if sniff == nil {
		t.Fatal("fixture has no generated sniff rule")
	}
	// Reproduce templates already polluted by repeated saves of runtime data.
	source := map[string]any{"route": map[string]any{"final": "direct", "rules": []any{
		sniff, sniff, sniff,
		map[string]any{"domain_suffix": []string{"example.com"}, "outbound": "direct", "comment": "keep"},
		map[string]any{"action": "route", "outbound": "direct"},
	}}}
	raw, _ := json.Marshal(source)
	if err := singBoxSettingService.SetSingBoxConfigTemplate(string(raw)); err != nil {
		t.Fatal(err)
	}
	for cycle := 0; cycle < 4; cycle++ {
		cfg, err = svc.GetConfig()
		if err != nil {
			t.Fatal(err)
		}
		if got := len(singBoxRouteRules(cfg.Route)); got != 3 {
			t.Fatalf("cycle %d runtime rules = %d, want one sniff plus two user rules", cycle, got)
		}
		data, err := cfg.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(svc.ProcessConfigPath()), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(svc.ProcessConfigPath(), data, 0600); err != nil {
			t.Fatal(err)
		}
		editor, err := svc.GetEditorConfig(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		route := editor.Config["route"].(map[string]any)
		rules := singBoxRouteRules(route)
		if len(rules) != 2 {
			t.Fatalf("cycle %d leaked runtime rules into editor: %#v", cycle, rules)
		}
		if rules[0].(map[string]any)["comment"] != "keep" || rules[1].(map[string]any)["outbound"] != "direct" {
			t.Fatalf("user policies changed: %#v", rules)
		}
		raw, _ = json.Marshal(editor.Config)
		if err := singBoxSettingService.SetSingBoxConfigTemplate(string(raw)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSingBoxEditorDoesNotImportStaleDiskRoute(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	svc := &SingBoxService{}
	if err := os.MkdirAll(filepath.Dir(svc.ProcessConfigPath()), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(svc.ProcessConfigPath(), []byte(`{"route":{"rules":[{"outbound":"stale"}]}}`), 0600); err != nil {
		t.Fatal(err)
	}
	editor, err := svc.GetEditorConfig(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range singBoxRouteRules(editor.Config["route"].(map[string]any)) {
		if raw.(map[string]any)["outbound"] == "stale" {
			t.Fatal("stale disk policy became editable")
		}
	}
}
