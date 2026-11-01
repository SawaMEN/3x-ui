package singbox

import "testing"

func TestNativePanelEndpointPreservesCascade(t *testing.T) {
	for _, kind := range []string{"awg", "warp", "tailscale", "openconnect", "openvpn-client", "masque-client", "tunnel_client"} {
		raw := map[string]any{"protocol": "singbox:" + kind, "tag": "endpoint", "settings": map[string]any{"detour": "hop", "custom_option": "keep"}}
		endpoint, handled, err := TranslatePanelEndpoint(raw)
		if err != nil || !handled || endpoint["type"] != kind || endpoint["detour"] != "hop" || endpoint["custom_option"] != "keep" {
			t.Fatalf("%s: endpoint=%v handled=%v err=%v", kind, endpoint, handled, err)
		}
		endpoint["custom_option"] = "changed"
		if raw["settings"].(map[string]any)["custom_option"] != "keep" {
			t.Fatal("stored settings mutated")
		}
	}
	if _, handled, err := TranslatePanelEndpoint(map[string]any{"protocol": "singbox:anytls"}); err != nil || handled {
		t.Fatalf("ordinary outbound classified as endpoint: %v", err)
	}
}
