package hiddify

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestInboundTCPFinalMaskTranslation(t *testing.T) {
	for _, network := range []string{"tcp", "ws", "grpc", "xhttp"} {
		t.Run(network, func(t *testing.T) {
			raw := vlessInbound()
			stream := object(raw, "streamSettings")
			stream["network"] = network
			delete(stream, "xhttpSettings")
			masks := map[string]any{"tcp": []any{map[string]any{"type": "fragment", "settings": map[string]any{"packets": "tlshello", "length": "10-20", "delay": "0"}}}}
			stream["finalmask"] = masks
			before, _ := json.Marshal(raw)
			out, err := TranslateInbound(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(out["final_mask"], masks) {
				t.Fatalf("lost mask: %#v", out)
			}
			after, _ := json.Marshal(raw)
			if string(before) != string(after) {
				t.Fatal("mutated saved Xray settings")
			}
		})
	}
}

func TestInboundTCPFinalMaskRejectsIncompatibleOptions(t *testing.T) {
	for _, tc := range []struct {
		name                    string
		value                   any
		network, security, want string
	}{
		{"bad object", "tcp", "tcp", "none", "must be an object"},
		{"bad array", map[string]any{"tcp": "bad"}, "tcp", "none", "must be an array"},
		{"unknown mask", map[string]any{"tcp": []any{map[string]any{"type": "unknown"}}}, "tcp", "none", "unsupported finalmask"},
		{"UDP transport", map[string]any{"tcp": []any{map[string]any{"type": "fragment"}}}, "kcp", "none", "cannot wrap"},
		{"REALITY", map[string]any{"tcp": []any{map[string]any{"type": "fragment"}}}, "tcp", "reality", "incompatible with REALITY"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := vlessInbound()
			stream := object(raw, "streamSettings")
			stream["network"], stream["security"], stream["finalmask"] = tc.network, tc.security, tc.value
			_, err := TranslateInbound(raw)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestInboundTCPFinalMaskOtherTCPProtocols(t *testing.T) {
	for _, protocol := range []string{"shadowsocks", "http", "socks", "mixed", "trojan", "vmess"} {
		t.Run(protocol, func(t *testing.T) {
			raw := map[string]any{"protocol": protocol, "tag": "masked", "listen": "127.0.0.1", "port": 45971,
				"settings":       map[string]any{"method": "chacha20-ietf-poly1305", "password": "secret", "clients": []any{map[string]any{"email": "alice", "id": "11111111-1111-4111-8111-111111111111", "password": "secret"}}},
				"streamSettings": map[string]any{"network": "tcp", "finalmask": map[string]any{"tcp": []any{map[string]any{"type": "fragment", "settings": map[string]any{"packets": "1-3", "length": "1-4", "delay": "0"}}}}},
			}
			out, err := TranslateInbound(raw)
			if err != nil {
				t.Fatal(err)
			}
			if out["final_mask"] == nil {
				t.Fatal("lost TCP FinalMask")
			}
		})
	}
}
