package singbox

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestOutboundInputNativeDialects(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(`{"type":" VLESS ","tag":"proxy","server":"127.0.0.1","server_port":443,"uuid":"2a547fe4-5b0f-4a84-845c-131464875bf8","transport":{"type":"XHTTP","path":"/"}}`), &raw); err != nil {
		t.Fatal(err)
	}
	result, err := TranslateXrayOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := normalizeOutboundsForRuntime([]map[string]any{result})
	if err != nil {
		t.Fatal(err)
	}
	if normalized[0]["type"] != "vless" || normalized[0]["transport"].(map[string]any)["type"] != "xhttp" {
		t.Fatalf("not normalized: %#v", result)
	}
	if raw["type"] != " VLESS " {
		t.Fatal("input mutated")
	}
	for _, field := range []string{"settings", "streamSettings", "mux", "proxySettings"} {
		for _, value := range []any{true, "broken", []any{1}, 12.5} {
			t.Run(field+fmt.Sprint(value), func(t *testing.T) {
				_, err := NormalizeOutboundInput(map[string]any{"protocol": "freedom", "tag": "p", field: value})
				if err == nil {
					t.Fatal("invalid object accepted")
				}
			})
		}
	}
}

func TestPanelIntegerBoundaryMatrix(t *testing.T) {
	fields := []string{"port", "serverPort", "server_port", "alterId", "mtu", "keepAlive", "maxEarlyData", "maxTimeDiff", "udpIdleTimeout", "up_mbps", "down_mbps", "minPacketSize", "maxPacketSize"}
	invalid := []any{-1, -999, 0.5, 443.9, 1e20, math.Inf(1), math.Inf(-1), math.NaN(), "1.5", "-1", "65536x", "", true, false, []any{1}, map[string]any{"value": 1}}
	for _, field := range fields {
		for i, value := range invalid {
			t.Run(fmt.Sprintf("%s/invalid-%02d", field, i), func(t *testing.T) {
				err := ValidatePanelIntegerOptions(map[string]any{"settings": map[string]any{"servers": []any{map[string]any{field: value}}}})
				if err == nil || !strings.Contains(err.Error(), field) {
					t.Fatalf("invalid option accepted: %v", value)
				}
			})
		}
		for _, value := range []any{0, 443, float64(443), "443"} {
			if err := ValidatePanelIntegerOptions(map[string]any{field: value}); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, port := range []any{65536, float64(65536), "65536"} {
		_, err := TranslateXrayOutbound(map[string]any{"protocol": "http", "tag": "p", "settings": map[string]any{"servers": []any{map[string]any{"address": "127.0.0.1", "port": port}}}})
		if err == nil {
			t.Fatal("out-of-range port accepted")
		}
	}
}
