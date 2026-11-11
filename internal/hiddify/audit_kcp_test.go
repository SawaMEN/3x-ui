package hiddify

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestAuditKCPNativeMaskContract(t *testing.T) {
	legacy := func(settings map[string]any) map[string]any {
		return map[string]any{"type": "mkcp-legacy", "settings": settings}
	}
	selected := []any{legacy(map[string]any{"value": "selected"})}
	for _, tc := range []struct {
		name    string
		options any
		mask    any
		want    []any
		invalid bool
	}{
		{name: "native numeric options", options: map[string]any{"mtu": 1350, "tti": 20}},
		{name: "seed", options: map[string]any{"seed": "secret"}, want: []any{legacy(map[string]any{"value": "secret"})}},
		{name: "header alias", options: map[string]any{"header": map[string]any{"type": "wechat-video"}}, want: []any{legacy(map[string]any{"value": ""}), legacy(map[string]any{"header": "wechat"})}},
		{name: "empty legacy seed", options: map[string]any{"seed": ""}, want: []any{legacy(map[string]any{"value": ""})}},
		{name: "explicit masks", mask: map[string]any{"udp": selected}, want: selected},
		{name: "explicit masks with empty legacy defaults", options: map[string]any{"seed": "", "header": map[string]any{"type": "none"}}, mask: map[string]any{"udp": selected}, want: selected},
		{name: "conflict", options: map[string]any{"seed": "secret"}, mask: map[string]any{"udp": selected}, invalid: true},
		{name: "malformed settings", options: "not-an-object", invalid: true},
		{name: "malformed masks", mask: map[string]any{"udp": "not-an-array"}, invalid: true},
		{name: "unsupported header", options: map[string]any{"header": map[string]any{"type": "unknown"}}, invalid: true},
	} {
		for _, inbound := range []bool{false, true} {
			direction := "outbounds"
			if inbound {
				direction = "inbounds"
			}
			t.Run(tc.name+"/"+direction, func(t *testing.T) {
				raw := vlessInbound()
				stream := object(raw, "streamSettings")
				stream["network"] = "kcp"
				if tc.options != nil {
					stream["kcpSettings"] = tc.options
				}
				if tc.mask != nil {
					stream["finalmask"] = tc.mask
				}
				if !inbound {
					raw["settings"] = map[string]any{"address": "127.0.0.1", "port": 443, "id": "11111111-1111-4111-8111-111111111111"}
				}
				before, _ := json.Marshal(raw)
				var out map[string]any
				var err error
				if inbound {
					out, err = TranslateInbound(raw)
				} else {
					out, err = TranslateOutbound(raw)
				}
				after, _ := json.Marshal(raw)
				if string(before) != string(after) {
					t.Fatal("translation mutated stored source settings")
				}
				if tc.invalid || inbound && len(tc.want) > 0 {
					if err == nil {
						t.Fatal("unsupported configuration was serialized")
					}
					if !tc.invalid && !strings.Contains(err.Error(), "UDP listener mask support") {
						t.Fatalf("missing actionable unsupported-inbound error: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				if len(tc.want) == 0 && out["final_mask"] != nil {
					t.Fatalf("native mKCP acquired unexpected masks: %#v", out["final_mask"])
				}
				if len(tc.want) != 0 && !reflect.DeepEqual(object(out, "final_mask")["udp"], tc.want) {
					t.Fatalf("mask chain changed: %#v", out["final_mask"])
				}
				checkProtectionConfig(t, direction, out)
			})
		}
	}
}
