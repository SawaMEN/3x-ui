package amneziawg

import (
	"encoding/json"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/util/wireguard"
)

func nativeOutboundFixture(t *testing.T) map[string]any {
	t.Helper()
	priv, pub, err := wireguard.GenerateWireguardKeypair()
	if err != nil {
		t.Fatal(err)
	}
	return map[string]any{"protocol": "amneziawg", "tag": "awg-egress", "proxySettings": map[string]any{"tag": "upstream"}, "settings": map[string]any{"native": true, "secretKey": priv, "address": []string{"10.8.1.2/32"}, "s4": 12, "jc": 4, "jmin": 10, "jmax": 20, "i1": "<r 32>", "peers": []any{map[string]any{"publicKey": pub, "endpoint": "[2001:db8::1]:51820", "allowedIPs": []string{"0.0.0.0/0"}, "keepAlive": 25}}}}
}

func TestNativeOutboundPreservesProtocolAndCascade(t *testing.T) {
	fixture := nativeOutboundFixture(t)
	raw, _ := json.Marshal(fixture)
	got, err := NativeOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got["tag"] != "awg-egress" || got["protocol"] != "singbox:awg" || got["proxySettings"] == nil {
		t.Fatalf("lost routing identity: %v", got)
	}
	s := got["settings"].(map[string]any)
	if s["mtu"] != 1408 || s["awg"].(map[string]any)["i1"] != "<r 32>" {
		t.Fatalf("lost AWG options: %v", s)
	}
	peer := s["peers"].([]map[string]any)[0]
	if peer["address"] != "2001:db8::1" || peer["port"] != 51820 || peer["persistent_keepalive_interval"] != 25 {
		t.Fatalf("lost peer options: %v", peer)
	}
}

func TestNativeOutboundRejectsBridgeSpecificSettings(t *testing.T) {
	for key, value := range map[string]any{"dns": "1.1.1.1", "listenPort": 51820} {
		fixture := nativeOutboundFixture(t)
		fixture["settings"].(map[string]any)[key] = value
		raw, _ := json.Marshal(fixture)
		if _, err := NativeOutbound(raw); err == nil {
			t.Fatalf("silently discarded %s", key)
		}
	}
}
