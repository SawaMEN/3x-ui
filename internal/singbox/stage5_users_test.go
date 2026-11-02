package singbox

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStage5TranslatorOmitsExplicitlyDisabledClients(t *testing.T) {
	for _, protocol := range []string{"vless", "vmess", "trojan", "shadowsocks", "hysteria", "hysteria2", "tuic", "naive", "snell", "anytls", "shadowtls", "http", "socks", "mixed"} {
		t.Run(protocol, func(t *testing.T) {
			clients := []any{
				map[string]any{"id": "enabled-id", "email": "enabled", "password": "enabled-password", "auth": "enabled-auth", "enable": true},
				map[string]any{"id": "disabled-id", "email": "disabled", "password": "disabled-password", "auth": "disabled-auth", "enable": false},
				map[string]any{"id": "legacy-id", "email": "legacy", "password": "legacy-password", "auth": "legacy-auth"},
			}
			settings := map[string]any{"clients": clients}
			before, _ := json.Marshal(settings)
			out := map[string]any{"tag": "test"}
			if err := translateUsers(out, protocol, settings); err != nil {
				t.Fatal(err)
			}
			data, _ := json.Marshal(out)
			if strings.Contains(string(data), "disabled") || !strings.Contains(string(data), "enabled") || !strings.Contains(string(data), "legacy") {
				t.Fatalf("wrong user selection: %s", data)
			}
			users := out["users"].([]map[string]any)
			if len(users) != 2 {
				t.Fatalf("user count=%d", len(users))
			}
			if protocol == "http" || protocol == "socks" || protocol == "mixed" {
				if users[0]["username"] != "enabled" {
					t.Fatalf("lost proxy username: %+v", users)
				}
			}
			after, _ := json.Marshal(settings)
			if string(before) != string(after) {
				t.Fatal("translator changed saved clients")
			}
		})
	}
}

func TestStage5DisabledProxyClientsCannotBecomeAnonymous(t *testing.T) {
	for _, protocol := range []string{"http", "socks", "mixed"} {
		out := map[string]any{}
		if err := translateUsers(out, protocol, map[string]any{"clients": []any{map[string]any{"email": "disabled", "password": "secret", "enable": false}}}); err == nil {
			t.Fatalf("%s became unauthenticated", protocol)
		}
	}
}
