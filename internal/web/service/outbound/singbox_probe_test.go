package outbound

import "testing"

func TestExtractNativeSingBoxOutboundEndpoint(t *testing.T) {
	got := extractOutboundEndpoints(map[string]any{
		"type":        "vless",
		"tag":         "native-vless",
		"server":      "2001:db8::10",
		"server_port": 443,
	})
	if len(got) != 1 || got[0] != "[2001:db8::10]:443" {
		t.Fatalf("endpoints = %#v, want [2001:db8::10]:443", got)
	}
}

func TestNativeSingBoxGroupHasNoDirectEndpoint(t *testing.T) {
	got := extractOutboundEndpoints(map[string]any{
		"type":      "urltest",
		"tag":       "auto",
		"outbounds": []any{"one", "two"},
	})
	if len(got) != 0 {
		t.Fatalf("endpoints = %#v, want none", got)
	}
}

func TestNativeSingBoxUDPOutboundUsesCoreProbe(t *testing.T) {
	for _, typeName := range []string{"hysteria2", "tuic", "wireguard"} {
		t.Run(typeName, func(t *testing.T) {
			ob := map[string]any{"type": typeName, "tag": typeName}
			if !outboundTransportIsUDP(ob) {
				t.Fatalf("outbound %q was not classified as UDP based", typeName)
			}
		})
	}
}
