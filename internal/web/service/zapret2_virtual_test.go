package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
	"github.com/SawaMEN/3x-ui/v3/internal/zapret2"
)

func testInstallZapret2(t *testing.T) {
	t.Helper()
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	for _, file := range []string{zapret2.BinaryPath(), filepath.Join(zapret2.Directory(), "lua", "zapret-lib.lua")} {
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestVirtualZapret2XrayRoutingIsScopedToLocalSOCKSInbound(t *testing.T) {
	testInstallZapret2(t)
	cfg := &xray.Config{
		InboundConfigs: []xray.InboundConfig{
			{Tag: "phone-direct", Port: 9443, Protocol: "vless"},
			{Tag: "phone-zapret", Port: 8443, Protocol: "vless"},
		},
		OutboundConfigs: []byte(`[
			{"tag":"zapret-rf", "protocol":"zapret2", "settings":{"port":15900,"profile":"safe","tcp":true}},
			{"tag":"direct", "protocol":"freedom", "settings":{}}
		]`),
		RouterConfig: []byte(`{"rules":[{"type":"field","inboundTag":["phone-zapret"],"outboundTag":"zapret-rf"}]}`),
	}
	if err := injectXrayZapret2(cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Zapret2Plans) != 1 || cfg.Zapret2Plans[0].Mark == 0 {
		t.Fatalf("invalid zapret2 plan: %#v", cfg.Zapret2Plans)
	}
	var outs []map[string]any
	if err := json.Unmarshal(cfg.OutboundConfigs, &outs); err != nil {
		t.Fatal(err)
	}
	if len(outs) != 3 {
		t.Fatalf("expected direct + loopback SOCKS + marked direct, got %#v", outs)
	}
	if outs[0]["tag"] != "direct" || outs[1]["tag"] != "zapret-rf" {
		t.Fatalf("existing routes unexpectedly altered: %#v", outs)
	}
	listenTag, directTag := zapret2Tags("zapret-rf")
	if len(cfg.InboundConfigs) != 3 || cfg.InboundConfigs[2].Tag != listenTag {
		t.Fatalf("zapret2 must listen only on its own local SOCKS inbound: %#v", cfg.InboundConfigs)
	}
	var route struct {
		Rules []struct {
			Inbound  []string `json:"inboundTag"`
			Outbound string   `json:"outboundTag"`
		} `json:"rules"`
	}
	if err := json.Unmarshal(cfg.RouterConfig, &route); err != nil {
		t.Fatal(err)
	}
	if len(route.Rules) != 2 || route.Rules[0].Inbound[0] != listenTag || route.Rules[0].Outbound != directTag {
		t.Fatalf("missing loopback-specific marked egress: %#v", route.Rules)
	}
	if route.Rules[1].Inbound[0] != "phone-zapret" || route.Rules[1].Outbound != "zapret-rf" {
		t.Fatalf("user's selected inbound route was not preserved: %#v", route.Rules)
	}
}

func TestVirtualZapret2SingBoxRoutingPreservesExistingRules(t *testing.T) {
	p, err := zapret2.Parse("z2", map[string]any{"port": 15911, "profile": "balanced"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := singbox.NewConfig()
	cfg.Outbounds = append(cfg.Outbounds, map[string]any{"type": "socks", "tag": "z2", "server": "127.0.0.1", "server_port": 15911})
	cfg.Route["rules"] = []map[string]any{{"inbound": []string{"phone-direct"}, "outbound": "direct"}}
	cfg.Zapret2Plans = []zapret2.Plan{p}
	cfg.Route["final"] = "z2"
	if err := injectSingBoxZapret2(cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Route["final"] != "direct" {
		t.Fatalf("zapret2 unexpectedly became default outbound: %v", cfg.Route["final"])
	}
	rules := singBoxRouteRules(cfg.Route)
	if len(rules) != 2 {
		t.Fatalf("expected 2 scoped routing rules, got %v", rules)
	}
	first, ok := rules[0].(map[string]any)
	if !ok || first["outbound"] != "__xui_zapret2_direct_z2" {
		t.Fatalf("missing highest priority local SOCKS route: %#v", rules[0])
	}
	second, ok := rules[1].(map[string]any)
	if !ok || second["outbound"] != "direct" {
		t.Fatalf("regular direct route was changed: %#v", rules[1])
	}
	for _, outbound := range cfg.Outbounds {
		if outbound["tag"] == "__xui_zapret2_direct_z2" && outbound["routing_mark"] != cfg.Zapret2Plans[0].Mark {
			t.Fatalf("missing dedicated mark on zapret2 direct: %v", outbound)
		}
	}
}

func TestVirtualZapret2RequiresOfficialInstallation(t *testing.T) {
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	cfg := &xray.Config{OutboundConfigs: []byte(`[{"tag":"z2","protocol":"zapret2","settings":{"port":15900}}]`)}
	if err := injectXrayZapret2(cfg); err == nil {
		t.Fatal("uninstalled zapret2 must not be added as outbound")
	}
}

func TestNativeZapret2TemplateTranslation(t *testing.T) {
	testInstallZapret2(t)
	cfg := singbox.NewConfig()
	cfg.Outbounds = []map[string]any{
		{"type": "zapret2", "tag": "z2", "settings": map[string]any{"port": 15912}},
		{"type": "direct", "tag": "direct"},
	}
	if err := translateNativeZapret2(cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Zapret2Plans) != 1 || cfg.Outbounds[0]["tag"] != "direct" || cfg.Outbounds[1]["type"] != "socks" {
		t.Fatalf("unexpected virtual outbound translation: %#v", cfg.Outbounds)
	}
	if err := injectSingBoxZapret2(cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Zapret2Plans) != 1 || cfg.Zapret2Plans[0].Options.Port != 15912 {
		t.Fatal("native settings were lost")
	}
}
