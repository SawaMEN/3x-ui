package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestWARPMASQUEJSONSavePersistsRegistrationCache(t *testing.T) {
	svc, _, _ := stage5ProcessFixture(t)
	if err := svc.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	candidate, err := svc.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	candidate.Outbounds = append(candidate.Outbounds, map[string]any{"type": "masque", "tag": "warp-masque"})
	candidate.Experimental = map[string]any{"cache_file": map[string]any{"enabled": false, "path": "/tmp/masque-test.db", "cache_id": "panel", "store_fakeip": true}, "clash_api": map[string]any{"external_controller": "127.0.0.1:19091"}}
	data, err := json.Marshal(candidate)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(data)
	if err := svc.SaveTemplate(context.Background(), raw); err != nil {
		t.Fatal(err)
	}
	saved, err := singBoxSettingService.GetSingBoxConfigTemplate()
	if err != nil {
		t.Fatal(err)
	}
	var cfg singbox.Config
	if err := json.Unmarshal([]byte(saved), &cfg); err != nil {
		t.Fatal(err)
	}
	cache, ok := cfg.Experimental["cache_file"].(map[string]any)
	if !ok || cache["enabled"] != true || cache["store_masque_config"] != true || cache["path"] != "/tmp/masque-test.db" || cache["cache_id"] != "panel" || cache["store_fakeip"] != true {
		t.Fatalf("cache settings lost: %#v", cfg.Experimental)
	}
	if cfg.Experimental["clash_api"].(map[string]any)["external_controller"] != "127.0.0.1:19091" || svc.IsRunning() {
		t.Fatal("save changed unrelated settings or started the core")
	}
}

func TestStoredWARPMASQUETemplateEnablesCache(t *testing.T) {
	cfg := singbox.NewConfig()
	if err := (&SingBoxService{}).applyNativeTemplateValue(cfg, nil, `{"outbounds":[{"type":"masque","tag":"legacy-warp"}]}`); err != nil {
		t.Fatal(err)
	}
	cache, ok := cfg.Experimental["cache_file"].(map[string]any)
	if !ok || cache["enabled"] != true || cache["store_masque_config"] != true {
		t.Fatalf("legacy WARP cache not enabled: %#v", cfg.Experimental)
	}
}

func TestMASQUEClientDoesNotEnableWARPCache(t *testing.T) {
	raw := `{"endpoints":[{"type":"masque-client","tag":"private-vpn"}],"experimental":{"cache_file":{"enabled":false}}}`
	normalized, err := normalizeSingBoxTemplate(raw)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(normalized, "store_masque_config") {
		t.Fatal("CONNECT-IP client incorrectly enabled WARP registration cache")
	}
	for _, raw := range []string{
		`{"outbounds":[{"type":"masque"}],"experimental":{"cache_file":"invalid"}}`,
		`{"outbounds":[{"type":"masque"}],"experimental":[]}`,
	} {
		if _, err := normalizeSingBoxTemplate(raw); err == nil {
			t.Fatalf("accepted invalid cache: %s", raw)
		}
	}
}

func TestGeneratedWARPMASQUEEnablesCacheWithoutNativeTemplate(t *testing.T) {
	cfg := singbox.NewConfig()
	cfg.Outbounds = append(cfg.Outbounds, map[string]any{"type": "masque", "tag": "shared-warp"})
	if err := (&SingBoxService{}).applyNativeTemplateValue(cfg, nil, ""); err != nil {
		t.Fatal(err)
	}
	cache, ok := cfg.Experimental["cache_file"].(map[string]any)
	if !ok || cache["store_masque_config"] != true {
		t.Fatalf("shared config has no registration cache: %#v", cfg.Experimental)
	}
}

func TestWARPMASQUEUsesStableCachePathWhenUnspecified(t *testing.T) {
	cfg := singbox.NewConfig()
	cfg.Outbounds = append(cfg.Outbounds, map[string]any{"type": "masque", "tag": "warp-masque"})
	if err := ensureWARPMASQUECache(cfg); err != nil {
		t.Fatal(err)
	}
	cache := cfg.Experimental["cache_file"].(map[string]any)
	want := filepath.Join(filepath.Dir(singbox.GetConfigPath()), "cache.db")
	if got := cache["path"]; got != want {
		t.Fatalf("cache path = %v, want %q", got, want)
	}
	if err := ensureWARPMASQUECache(cfg); err != nil {
		t.Fatal(err)
	}
	if cache["path"] != want {
		t.Fatal("cache path changed on repeated apply")
	}
}
