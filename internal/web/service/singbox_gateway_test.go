package service

import (
	"encoding/json"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"reflect"
	"testing"
)

const gatewayNativeInbound = `[{"type":"tproxy","tag":"in-tproxy","listen":"0.0.0.0","listen_port":52345}]`

func TestSingBoxGatewayReachesGeneratedConfig(t *testing.T) {
	setupBulkDB(t)
	if err := singBoxSettingService.SetSingBoxConfigTemplate(`{"inbounds":` + gatewayNativeInbound + `,"route":{"rules":[{"inbound":["in-tproxy"],"action":"sniff","sniffer":["http","tls","quic"]}]}}`); err != nil {
		t.Fatal(err)
	}
	cfg, err := (&SingBoxService{}).GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, inbound := range cfg.Inbounds {
		if inbound["tag"] == "in-tproxy" {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("Gateway listener missing/duplicated in actual config: %#v", cfg.Inbounds)
	}
	if cfg.Route == nil || reflect.ValueOf(cfg.Route["rules"]).Len() == 0 {
		t.Fatal("Gateway sniff route missing")
	}
}
func TestSingBoxGatewayTemplateInboundIsolation(t *testing.T) {
	cfg := &singbox.Config{}
	if err := mergeSingBoxGatewayInbound(cfg, json.RawMessage(`[{"type":"mixed","tag":"disabled-panel","listen_port":1080}]`)); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Inbounds) != 0 {
		t.Fatal("ordinary template inbound revived")
	}
	for _, raw := range []string{`[{"type":"mixed","tag":"in-tproxy","listen_port":52345}]`, gatewayNativeInbound[:len(gatewayNativeInbound)-1] + `,` + gatewayNativeInbound[1:], `[{"type":"tproxy","tag":"in-tproxy","listen":"0.0.0.0","listen_port":52345,"network":"tcp"}]`} {
		if err := mergeSingBoxGatewayInbound(cfg, json.RawMessage(raw)); err == nil {
			t.Fatalf("bad Gateway accepted: %s", raw)
		}
	}
	cfg.Inbounds = []map[string]any{{"tag": "user", "listen_port": 52345}}
	if err := mergeSingBoxGatewayInbound(cfg, json.RawMessage(gatewayNativeInbound)); err == nil {
		t.Fatal("managed port collision ignored")
	}
}
func TestSingBoxGatewayRoutePreservesGeneratedRoutes(t *testing.T) {
	base := json.RawMessage(`{"final":"proxy","auto_detect_interface":true,"rules":[{"domain_suffix":["example.com"],"outbound":"direct"}]}`)
	patch := json.RawMessage(`{"rules":[{"inbound":["in-tproxy"],"action":"sniff","sniffer":["http","tls","quic"]}]}`)
	raw, ok := mergeGatewayOnlyRoute(base, patch)
	if !ok {
		t.Fatal("Gateway route not merged")
	}
	var route map[string]any
	if err := json.Unmarshal(raw, &route); err != nil {
		t.Fatal(err)
	}
	if route["final"] != "proxy" || route["auto_detect_interface"] != true || len(route["rules"].([]any)) != 2 {
		t.Fatalf("generated routing lost: %s", raw)
	}
	if _, ok := mergeGatewayOnlyRoute(base, json.RawMessage(`{"final":"operator","rules":[]}`)); ok {
		t.Fatal("explicit operator routing overridden")
	}
}
