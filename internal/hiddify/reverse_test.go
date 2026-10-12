package hiddify

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestReverseTranslationAndPortal(t *testing.T) {
	raw := map[string]any{"protocol": "vless", "tag": "server", "port": 443, "settings": map[string]any{"clients": []any{map[string]any{"id": "11111111-1111-4111-8111-111111111111", "email": "reverse", "reverse": map[string]any{"tag": "portal"}}, map[string]any{"id": "22222222-2222-4222-8222-222222222222", "email": "ordinary"}}}}
	out, err := TranslateInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	users := out["users"].([]map[string]any)
	if users[0]["reverse"].(map[string]any)["tag"] != "portal" || users[1]["reverse"] != nil {
		t.Fatalf("reverse mapped to wrong user: %v", users)
	}
	cfg := singbox.NewConfig()
	cfg.Inbounds = []map[string]any{out}
	if err = AddReversePortals(cfg); err != nil {
		t.Fatal(err)
	}
	last := cfg.Outbounds[len(cfg.Outbounds)-1]
	if last["type"] != "vless-reverse" || last["tag"] != "portal" {
		t.Fatal(last)
	}
	// Do not mutate the stored JSON while extracting extensions.
	if raw["settings"].(map[string]any)["clients"].([]any)[0].(map[string]any)["reverse"] == nil {
		t.Fatal("source JSON mutated")
	}
}

func TestReverseOutboundSniffing(t *testing.T) {
	raw := map[string]any{"protocol": "vless", "tag": "reverse-bridge", "settings": map[string]any{"vnext": []any{map[string]any{"address": "127.0.0.1", "port": 443, "users": []any{map[string]any{"id": "11111111-1111-4111-8111-111111111111", "reverse": map[string]any{"tag": "bridge", "sniffing": map[string]any{"enabled": true, "routeOnly": true, "destOverride": []any{"http", "tls"}}}}}}}}}
	out, err := TranslateOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	cfg := singbox.NewConfig()
	cfg.Outbounds = append(cfg.Outbounds, out)
	if err = AddReversePortals(cfg); err != nil {
		t.Fatal(err)
	}
	rules := cfg.Route["rules"].([]any)
	if len(rules) != 1 || rules[0].(map[string]any)["action"] != "sniff" {
		t.Fatal(rules)
	}
	if out["reverse"].(map[string]any)["sniffing"] != nil {
		t.Fatal("legacy sniffing leaked into native runtime")
	}
	cfg.Route["final"] = "reverse-bridge"
	if err = AddReversePortals(cfg); err == nil {
		t.Fatal("reverse-only default route accepted")
	}
}

func TestReverseRejectInvalidOptionsAndCollisions(t *testing.T) {
	for _, reverse := range []any{"portal", map[string]any{"tag": ""}, map[string]any{"tag": " portal"}, map[string]any{"tag": "portal", "unknown": true}} {
		raw := map[string]any{"protocol": "vless", "settings": map[string]any{"clients": []any{map[string]any{"id": "user", "reverse": reverse}}}}
		if _, err := TranslateInbound(raw); err == nil {
			t.Fatalf("invalid reverse accepted: %v", reverse)
		}
	}
	cfg := singbox.NewConfig()
	cfg.Inbounds = []map[string]any{{"type": "vless", "users": []map[string]any{{"reverse": map[string]any{"tag": "direct"}}}}}
	if err := AddReversePortals(cfg); err == nil {
		t.Fatal("portal collided with ordinary outbound")
	}
}

func TestNativeReverseImportAndExplicitPortal(t *testing.T) {
	out, err := TranslateOutbound(map[string]any{"type": "vless", "tag": "remote", "server": "127.0.0.1", "server_port": 443, "uuid": "11111111-1111-4111-8111-111111111111", "reverse": map[string]any{"tag": "bridge"}})
	if err != nil {
		t.Fatal(err)
	}
	checkProtectionConfig(t, "outbounds", out)
	cfg := singbox.NewConfig()
	cfg.Inbounds = []map[string]any{{"type": "vless", "users": []map[string]any{{"reverse": map[string]any{"tag": "portal"}}}}}
	cfg.Outbounds = append(cfg.Outbounds, map[string]any{"type": "vless-reverse", "tag": "portal"})
	before := len(cfg.Outbounds)
	if err := AddReversePortals(cfg); err != nil {
		t.Fatal(err)
	}
	if len(cfg.Outbounds) != before {
		t.Fatal("explicit portal duplicated")
	}
	if err := AddReversePortals(cfg); err != nil {
		t.Fatal("portal generation is not idempotent", err)
	}
}
