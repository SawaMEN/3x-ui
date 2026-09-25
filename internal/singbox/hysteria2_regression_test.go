package singbox

import "testing"

func TestTranslateHysteria2OutboundPortHoppingOmitsServerPort(t *testing.T) {
	raw := map[string]any{
		"protocol": "hysteria",
		"tag":      "hy2-hop",
		"settings": map[string]any{
			"address": "example.com",
		},
		"streamSettings": map[string]any{
			"network":  "hysteria",
			"security": "tls",
			"hysteriaSettings": map[string]any{
				"version":      2,
				"auth":         "secret",
				"server_ports": []any{"20000:30000", "40000"},
				"hop_interval": "30s",
			},
		},
	}

	got, err := TranslateXrayOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["server_port"]; ok {
		t.Fatalf("server_port conflicts with server_ports and must be omitted: %#v", got)
	}
	ports, ok := got["server_ports"].([]string)
	if !ok || len(ports) != 2 || ports[0] != "20000:30000" || ports[1] != "40000" {
		t.Fatalf("unexpected server_ports: %#v", got["server_ports"])
	}
	if got["hop_interval"] != "30s" {
		t.Fatalf("unexpected hop_interval: %#v", got["hop_interval"])
	}
}

func TestTranslateXrayWebSocketHostWithoutExistingHeaders(t *testing.T) {
	raw := map[string]any{
		"protocol": "vless",
		"tag":      "ws-host",
		"settings": map[string]any{
			"clients": []any{map[string]any{"id": "11111111-1111-1111-1111-111111111111"}},
		},
		"streamSettings": map[string]any{
			"network": "ws",
			"wsSettings": map[string]any{
				"host": "example.com",
			},
		},
	}

	got, err := TranslateXrayInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	transport, ok := got["transport"].(map[string]any)
	if !ok {
		t.Fatalf("missing websocket transport: %#v", got["transport"])
	}
	headers, ok := transport["headers"].(map[string]any)
	if !ok || headers["Host"] != "example.com" {
		t.Fatalf("websocket Host header was not preserved: %#v", transport["headers"])
	}
}
