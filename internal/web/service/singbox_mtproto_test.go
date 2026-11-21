package service

import (
	"slices"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestSingBoxMtprotoEgress(t *testing.T) {
	for _, endpoint := range []bool{false, true} {
		cfg := &singbox.Config{Route: map[string]any{"rules": []map[string]any{{"action": "reject"}}}}
		targets := []map[string]any{{"tag": "egress"}}
		if endpoint {
			cfg.Endpoints = targets
		} else {
			cfg.Outbounds = targets
		}
		ib := &model.Inbound{Enable: true, Protocol: model.MTProto, Tag: "telegram", Settings: `{"routeThroughXray":true,"routeXrayPort":32123,"outboundTag":"egress"}`}
		if err := injectSingBoxMtprotoEgress(cfg, ib); err != nil {
			t.Fatal(err)
		}
		if len(cfg.Inbounds) != 1 || cfg.Inbounds[0]["listen"] != "127.0.0.1" || cfg.Inbounds[0]["type"] != "socks" {
			t.Fatalf("bad bridge: %#v", cfg.Inbounds)
		}
		rules := cfg.Route["rules"].([]map[string]any)
		if len(rules) != 2 || rules[0]["outbound"] != "egress" {
			t.Fatalf("selected egress must precede template rules: %#v", rules)
		}
	}
	ib := &model.Inbound{Enable: true, Protocol: model.MTProto, Tag: "telegram", Settings: `{"routeThroughXray":true,"routeXrayPort":32123,"outboundTag":"missing"}`}
	if err := injectSingBoxMtprotoEgress(&singbox.Config{}, ib); err == nil {
		t.Fatal("missing target accepted")
	}
	ib.Settings = `{"routeThroughXray":true,"routeXrayPort":32123}`
	configured := map[string]any{"inbound": []string{ib.Tag}, "action": "route", "outbound": "egress"}
	cfg := &singbox.Config{Route: map[string]any{"rules": []map[string]any{configured}}}
	if err := injectSingBoxMtprotoEgress(cfg, ib); err != nil {
		t.Fatal(err)
	}
	if rules := singBoxRouteRules(cfg.Route); len(rules) != 1 || rules[0].(map[string]any)["outbound"] != "egress" || len(cfg.Inbounds) != 1 || cfg.Inbounds[0]["tag"] != ib.Tag {
		t.Fatal("unselected egress must use normal routing")
	}
	ib.Enable = false
	cfg = &singbox.Config{}
	if err := injectSingBoxMtprotoEgress(cfg, ib); err != nil || len(cfg.Inbounds) != 0 {
		t.Fatal("disabled inbound installed a bridge")
	}
}

func TestMtprotoWebDoesNotInstallCoreBridge(t *testing.T) {
	ib := mtprotoInbound("telegram-web", `{"routeThroughXray":true,"routeXrayPort":50000,"outboundTag":"missing","webProxy":{"enabled":true}}`)
	xray := egressTestConfig()
	before := len(xray.InboundConfigs)
	injectMtprotoEgress(xray, ib)
	if len(xray.InboundConfigs) != before {
		t.Fatal("WEB Proxy installed an unused Xray bridge")
	}
	native := &singbox.Config{}
	if err := injectSingBoxMtprotoEgress(native, ib); err != nil || len(native.Inbounds) != 0 {
		t.Fatalf("WEB Proxy must not change native routing: %v", err)
	}
}

func TestSingBoxMtprotoFallbackOutbounds(t *testing.T) {
	cfg := &singbox.Config{
		Route:     map[string]any{"rules": []map[string]any{}},
		Outbounds: []map[string]any{{"tag": "primary", "type": "socks"}},
		Endpoints: []map[string]any{{"tag": "reserve", "type": "awg"}},
	}
	ib := &model.Inbound{
		Id: 23, Enable: true, Protocol: model.MTProto, Tag: "telegram",
		Settings: `{"routeThroughXray":true,"routeXrayPort":32124,"outboundTag":"primary","fallbackOutboundTags":["reserve"]}`,
	}
	if err := injectSingBoxMtprotoEgress(cfg, ib); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 2 {
		t.Fatalf("expected the primary outbound plus one URLtest group, got %v", cfg.Outbounds)
	}
	group := cfg.Outbounds[1]
	if group["type"] != "urltest" || group["tag"] != "__telemt_route_auto_23" || group["url"] != "https://core.telegram.org/" {
		t.Fatalf("unexpected health-checked group: %#v", group)
	}
	if !slices.Equal(group["outbounds"].([]string), []string{"primary", "reserve"}) {
		t.Fatalf("URLtest group did not keep the AWG endpoint as its fallback: %#v", group)
	}
	rules := cfg.Route["rules"].([]map[string]any)
	if len(rules) != 1 || rules[0]["outbound"] != group["tag"] {
		t.Fatalf("MTProto egress must use the health-checked group: %#v", rules)
	}
	ib.Settings = `{"routeThroughXray":true,"routeXrayPort":32124,"outboundTag":"primary","fallbackOutboundTags":["missing"]}`
	if err := injectSingBoxMtprotoEgress(&singbox.Config{Outbounds: []map[string]any{{"tag": "primary"}}}, ib); err == nil {
		t.Fatal("missing fallback outbound must be rejected before publishing config")
	}
}
