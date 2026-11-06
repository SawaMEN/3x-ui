package service

import (
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
	cfg := &singbox.Config{}
	if err := injectSingBoxMtprotoEgress(cfg, ib); err != nil {
		t.Fatal(err)
	}
	if cfg.Route != nil || len(cfg.Inbounds) != 1 {
		t.Fatal("unselected egress must use normal routing")
	}
	ib.Enable = false
	cfg = &singbox.Config{}
	if err := injectSingBoxMtprotoEgress(cfg, ib); err != nil || len(cfg.Inbounds) != 0 {
		t.Fatal("disabled inbound installed a bridge")
	}
}

func TestSingBoxMtprotoFallbackOutbounds(t *testing.T) {
	cfg := &singbox.Config{
		Route: map[string]any{"rules": []map[string]any{}},
		Outbounds: []map[string]any{{"tag": "primary", "type": "socks"}, {"tag": "reserve", "type": "socks"}},
	}
	ib := &model.Inbound{
		Id: 23, Enable: true, Protocol: model.MTProto, Tag: "telegram",
		Settings: `{"routeThroughXray":true,"routeXrayPort":32124,"outboundTag":"primary","fallbackOutboundTags":["reserve"]}`,
	}
	if err := injectSingBoxMtprotoEgress(cfg, ib); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != 3 {
		t.Fatalf("expected one URLtest balancer, got %d", len(cfg.Outbounds))
	}
	group := cfg.Outbounds[2]
	if group["type"] != "urltest" || group["tag"] != "__telemt_route_auto_23" || group["url"] != "https://core.telegram.org/" {
		t.Fatalf("unexpected health-checked group: %#v", group)
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
