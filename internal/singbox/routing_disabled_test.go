package singbox

import "testing"

func TestTranslateRoutingSkipsDisabledCascadeRules(t *testing.T) {
	disabled := map[string]any{"type": "field", "enabled": false, "inboundTag": []any{"vless", "tuic", "anytls"}, "outboundTag": "direct"}
	cfg, err := TranslateXrayRouting(map[string]any{"rules": []any{disabled, map[string]any{"type": "field", "outboundTag": "cascade"}}})
	if err != nil {
		t.Fatal(err)
	}
	rules := cfg["rules"].([]map[string]any)
	if len(rules) != 1 || rules[0]["outbound"] != "cascade" {
		t.Fatalf("disabled rule overrides cascade: %v", rules)
	}
	if disabled["enabled"] != false {
		t.Fatal("source policy mutated")
	}
}

func TestTranslateRoutingSkipsDisabledUnsupportedMatchers(t *testing.T) {
	_, err := TranslateXrayRouting(map[string]any{"rules": []any{map[string]any{"enabled": false, "attrs": "unsupported", "outboundTag": "missing"}}})
	if err != nil {
		t.Fatal(err)
	}
}
