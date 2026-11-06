package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func seedProtocolCascade(t *testing.T, tag string) {
	t.Helper()
	sub := &model.OutboundSubscription{Enabled: true, LastFetchedOutbounds: `[{"tag":"cascade-upstream","protocol":"singbox:socks","settings":{"server":"127.0.0.1","server_port":19090}}]`}
	if err := database.GetDB().Create(sub).Error; err != nil {
		t.Fatal(err)
	}
	patch, _ := json.Marshal(map[string]any{"route": map[string]any{"final": "blocked", "rules": []any{map[string]any{"inbound": []string{tag}, "network": []string{"tcp", "udp"}, "action": "route", "outbound": "cascade-upstream"}}}})
	if err := singBoxSettingService.SetSingBoxConfigTemplate(string(patch)); err != nil {
		t.Fatal(err)
	}
}

func assertProtocolCascade(t *testing.T, cfg *singbox.Config, tag string) {
	t.Helper()
	if cfg.Route["final"] != "blocked" {
		t.Fatalf("fallback changed: %v", cfg.Route)
	}
	for _, raw := range singBoxRouteRules(cfg.Route) {
		rule, ok := raw.(map[string]any)
		if !ok || rule["outbound"] != "cascade-upstream" {
			continue
		}
		for _, inbound := range readInboundTags(rule["inbound"]) {
			if inbound == tag {
				return
			}
		}
	}
	t.Fatalf("%s lost its subscription route", tag)
}

func TestProtocolCascadeUnsupportedLegacyListeners(t *testing.T) {
	for _, protocol := range []model.Protocol{model.WireGuard, model.Tunnel} {
		t.Run(string(protocol), func(t *testing.T) {
			setupBulkDB(t)
			ib := &model.Inbound{Enable: true, Protocol: protocol, Tag: "legacy", Port: 19443, Settings: `{}`, StreamSettings: `{}`}
			if err := database.GetDB().Create(ib).Error; err != nil {
				t.Fatal(err)
			}
			if _, err := (&HiddifyConfigGenerator{}).Generate(); err == nil {
				t.Fatal("unsupported listener silently accepted")
			}
		})
	}
}

func TestProtocolCascadeTUN(t *testing.T) {
	setupBulkDB(t)
	t.Setenv("XUI_BIN_FOLDER", t.TempDir())
	ib := &model.Inbound{Enable: true, Protocol: model.Tun, Tag: "tun-cascade", Settings: `{"name":"cascade0","gateway":["10.60.0.1/30"]}`, StreamSettings: `{}`}
	if err := database.GetDB().Create(ib).Error; err != nil {
		t.Fatal(err)
	}
	seedProtocolCascade(t, ib.Tag)
	cfg, err := (&HiddifyConfigGenerator{}).Generate()
	if err != nil {
		t.Fatal(err)
	}
	assertProtocolCascade(t, cfg, ib.Tag)
	checkGeneratedHiddifyConfig(t, cfg)
	if fmt.Sprint(cfg.Inbounds[0]["type"]) != "tun" {
		t.Fatal("TUN listener lost")
	}
}
