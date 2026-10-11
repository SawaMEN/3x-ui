package hiddify

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestServerConfigReferenceFailures(t *testing.T) {
	for _, tc := range []struct{ name, patch, want string }{
		{"route final", `"route":{"final":"lost"}`, "route.final"},
		{"nested route", `"route":{"rules":[{"type":"logical","rules":[{"domain":"example.com","outbound":"lost"}]}]}`, "missing outbound"},
		{"DNS final", `"dns":{"servers":[{"type":"local","tag":"local"}],"final":"lost"}`, "dns.final"},
		{"DNS action", `"dns":{"servers":[{"type":"local","tag":"local"}],"rules":[{"domain":"example.com","server":"lost"}]}`, "missing DNS server"},
		{"resolver", `"route":{"default_domain_resolver":{"server":"lost"}}`, "default_domain_resolver"},
		{"rule set", `"route":{"rules":[{"rule_set":["lost"],"outbound":"direct"}]}`, "missing rule set"},
		{"DNS rule set", `"dns":{"rules":[{"rule_set":"lost","server":"local"}]}`, "missing rule set"},
		{"group member", `"outbounds":[{"type":"selector","tag":"select","outbounds":["lost"]}]`, "select.outbounds"},
		{"group default", `"outbounds":[{"type":"direct","tag":"direct"},{"type":"selector","tag":"select","outbounds":["direct"],"default":"lost"}]`, "not a member"},
		{"group cycle", `"outbounds":[{"type":"selector","tag":"a","outbounds":["b"]},{"type":"selector","tag":"b","outbounds":["a"]}]`, "dependency cycle"},
		{"detour cycle", `"outbounds":[{"type":"direct","tag":"a","detour":"a"}]`, "dependency cycle"},
		{"inbound detour", `"inbounds":[{"type":"shadowtls","tag":"outer","detour":"lost"}]`, "missing inbound"},
		{"duplicate listener", `"inbounds":[{"tag":"same"},{"tag":"same"}]`, "duplicate listener"},
		{"duplicate resolver", `"dns":{"servers":[{"tag":"same"},{"tag":"same"}]}`, "duplicate DNS server"},
		{"download detour", `"route":{"rule_set":[{"tag":"rules","download_detour":"lost"}]}`, "download_detour"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cfg map[string]any
			if err := json.Unmarshal([]byte(`{"outbounds":[{"type":"direct","tag":"direct"}],`+tc.patch+`}`), &cfg); err != nil {
				t.Fatal(err)
			}
			if err := ValidateConfigReferences(cfg); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %s, got %v", tc.want, err)
			}
		})
	}
}

func TestServerConfigShadowTLSDetourAndStableStats(t *testing.T) {
	cfg := singbox.NewConfig()
	cfg.Inbounds = []map[string]any{
		{"type": "shadowsocks", "tag": "inner", "users": []any{map[string]any{"name": "alice"}}},
		{"type": "shadowtls", "tag": "outer", "detour": "inner", "users": []map[string]any{{"name": "alice"}}},
		{"type": "mixed", "tag": "proxy", "users": []any{map[string]any{"username": "bob"}}},
	}
	// Peer metadata is native to the user's fork, not a sing-box inbound user.
	cfg.Endpoints = []map[string]any{{"type": "wireguard", "tag": "wg", "address": []string{"10.123.0.1/24"}, "private_key": "private", "peers": []any{map[string]any{"name": "alice", "public_key": "public", "allowed_ips": []string{"10.123.0.2/32"}}}}}
	cfg.Services = []map[string]any{{"type": "api"}}
	if err := FinalizeServerConfig(cfg); err != nil {
		t.Fatal(err)
	}
	if !cfg.Hiddify || len(cfg.Services) != 0 {
		t.Fatal("wrong server profile")
	}
	stats := cfg.Experimental["v2ray_api"].(map[string]any)["stats"].(map[string]any)
	if !reflect.DeepEqual(stats["users"], []string{"alice", "bob"}) {
		t.Fatalf("users = %v", stats["users"])
	}
	if !reflect.DeepEqual(stats["inbounds"], []string{"inner", "outer", "proxy", "wg"}) {
		t.Fatalf("tags = %v", stats["inbounds"])
	}
	if !reflect.DeepEqual(stats["outbounds"], []string{"block", "direct"}) {
		t.Fatalf("normalized outbound tags lost in stats: %v", stats["outbounds"])
	}
	before, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if err := FinalizeServerConfig(cfg); err != nil {
		t.Fatal(err)
	}
	after, err := cfg.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("server finalization is not deterministic")
	}
}
