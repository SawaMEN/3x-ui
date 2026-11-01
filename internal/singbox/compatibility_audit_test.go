package singbox

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestRoutingPreservesPortsAndAuthenticatedUsers(t *testing.T) {
	var raw map[string]any
	if err := json.Unmarshal([]byte(`{"rules":[{"port":"80,443,1000-2000","sourcePort":5353,"source":["192.0.2.0/24"],"user":["alice"],"outboundTag":"direct"}]}`), &raw); err != nil {
		t.Fatal(err)
	}
	got, err := TranslateXrayRouting(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"rules": []map[string]any{{
		"port": []uint16{80, 443}, "port_range": []string{"1000:2000"},
		"source_port": []uint16{5353}, "source_ip_cidr": []string{"192.0.2.0/24"},
		"auth_user": []string{"alice"}, "action": "route", "outbound": "direct",
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("routing = %#v, want %#v", got, want)
	}
}

func TestRoutingRejectsUnrepresentableMatchers(t *testing.T) {
	for _, tc := range []struct {
		field string
		value any
	}{
		{"port", "443,70000"},
		{"sourcePort", "2000-1000"},
		{"attrs", map[string]any{"method": "GET"}},
		{"sourceIP", []any{"geoip:cn"}},
		{"localPort", "443"},
	} {
		t.Run(tc.field, func(t *testing.T) {
			_, err := TranslateXrayRouting(map[string]any{"rules": []any{
				map[string]any{tc.field: tc.value, "outboundTag": "blocked"},
			}})
			if err == nil || !strings.Contains(err.Error(), tc.field) {
				t.Fatalf("error = %v, want diagnostic naming %s", err, tc.field)
			}
		})
	}
}

func TestStreamPreservesWebSocketHostAndEmptyRealityShortID(t *testing.T) {
	t.Run("websocket host", func(t *testing.T) {
		headers := map[string]any{"X-Token": "test", "Host": "old.example"}
		out := map[string]any{"tag": "proxy"}
		err := translateStream(out, "vless", map[string]any{
			"network": "ws", "wsSettings": map[string]any{"host": "cdn.example", "path": "/ws", "headers": headers},
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		transport := out["transport"].(map[string]any)
		got := transport["headers"].(map[string]any)
		if got["Host"] != "cdn.example" || got["X-Token"] != "test" || headers["Host"] != "old.example" {
			t.Fatalf("translated headers = %#v, original = %#v", got, headers)
		}
	})
	t.Run("empty reality short id", func(t *testing.T) {
		out := map[string]any{"tag": "proxy"}
		err := translateStream(out, "vless", map[string]any{
			"network": "tcp", "security": "reality", "realitySettings": map[string]any{
				"publicKey": "public", "shortId": "", "serverName": "example.com", "fingerprint": "chrome",
			},
		}, false)
		if err != nil {
			t.Fatal(err)
		}
		tls := out["tls"].(map[string]any)
		if tls["reality"].(map[string]any)["short_id"] != "" || !reflect.DeepEqual(tls["utls"], map[string]any{"enabled": true, "fingerprint": "chrome"}) {
			t.Fatalf("REALITY options = %#v", tls)
		}
	})
}

func TestStreamPreservesInlineTLSCertificate(t *testing.T) {
	out := map[string]any{"tag": "inbound"}
	err := translateStream(out, "trojan", map[string]any{
		"security": "tls", "tlsSettings": map[string]any{"certificates": []any{
			map[string]any{"certificate": []any{"cert-line-1", "cert-line-2"}, "key": []any{"key-line"}},
		}},
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	tls := out["tls"].(map[string]any)
	if !reflect.DeepEqual(tls["certificate"], []string{"cert-line-1", "cert-line-2"}) || !reflect.DeepEqual(tls["key"], []string{"key-line"}) {
		t.Fatalf("TLS options = %#v", tls)
	}
}

func TestRealityKeepsDirectionSpecificOptions(t *testing.T) {
	for _, inbound := range []bool{false, true} {
		out := map[string]any{"tag": "reality"}
		err := translateStream(out, "vless", map[string]any{
			"security": "reality", "realitySettings": map[string]any{
				"publicKey": "public", "privateKey": "private", "target": "example.com:443",
				"shortIds": []any{"", "abcd"},
			},
		}, inbound)
		if err != nil {
			t.Fatal(err)
		}
		reality := out["tls"].(map[string]any)["reality"].(map[string]any)
		if inbound {
			if reality["public_key"] != nil || !reflect.DeepEqual(reality["short_id"], []string{"", "abcd"}) {
				t.Fatalf("inbound REALITY = %#v", reality)
			}
		} else if reality["private_key"] != nil || reality["short_id"] != "" {
			t.Fatalf("outbound REALITY = %#v", reality)
		}
	}
}

func TestRoutingPreservesDomainAndIPConjunction(t *testing.T) {
	routing, err := TranslateXrayRouting(map[string]any{"rules": []any{map[string]any{
		"domain": []any{"full:example.com"}, "ip": []any{"192.0.2.0/24"}, "outboundTag": "direct",
	}}})
	if err != nil {
		t.Fatal(err)
	}
	rule := routing["rules"].([]map[string]any)[0]
	if rule["type"] != "logical" || rule["mode"] != "and" || rule["outbound"] != "direct" {
		t.Fatalf("rule = %#v", rule)
	}
	children := rule["rules"].([]map[string]any)
	if len(children) != 2 || children[0]["domain"] == nil || children[1]["ip_cidr"] == nil {
		t.Fatalf("logical rules = %#v", children)
	}
}
