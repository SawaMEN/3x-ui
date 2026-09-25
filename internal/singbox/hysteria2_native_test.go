package singbox

import (
	"strings"
	"testing"
)

func TestTranslateHysteria2InboundNativeSettings(t *testing.T) {
	raw := map[string]any{
		"protocol": "hysteria",
		"tag":      "hy2-native",
		"port":     443,
		"settings": map[string]any{
			"clients": []any{
				map[string]any{
					"email": "alice",
					"auth":  "secret",
				},
			},
		},
		"streamSettings": map[string]any{
			"network":  "hysteria",
			"security": "tls",
			"hysteriaSettings": map[string]any{
				"version":               2,
				"up":                    100,
				"down":                  250,
				"ignoreClientBandwidth": true,
				"bbrProfile":            "AGGRESSIVE",
				"obfs": map[string]any{
					"type":     "Salamander",
					"password": "obfs-secret",
				},
			},
			"tlsSettings": map[string]any{
				"serverName": "example.com",
			},
		},
	}

	got, err := TranslateXrayInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got["type"] != "hysteria2" {
		t.Fatalf("expected native sing-box hysteria2, got %#v", got["type"])
	}
	if got["up_mbps"] != 100 || got["down_mbps"] != 250 {
		t.Fatalf("bandwidth was not translated to Mbps: %#v", got)
	}
	if got["ignore_client_bandwidth"] != true {
		t.Fatalf("ignoreClientBandwidth was not translated: %#v", got)
	}
	if got["bbr_profile"] != "aggressive" {
		t.Fatalf("bbr_profile was not normalized: %#v", got)
	}
	obfs, ok := got["obfs"].(map[string]any)
	if !ok || obfs["type"] != "salamander" || obfs["password"] != "obfs-secret" {
		t.Fatalf("obfs was not normalized: %#v", got["obfs"])
	}
}

func TestTranslateHysteria2InboundRequiresTLS(t *testing.T) {
	raw := map[string]any{
		"protocol": "hysteria",
		"tag":      "hy2-no-tls",
		"port":     443,
		"settings": map[string]any{
			"clients": []any{map[string]any{"auth": "secret"}},
		},
		"streamSettings": map[string]any{
			"network": "hysteria",
			"hysteriaSettings": map[string]any{
				"version": 2,
			},
		},
	}

	_, err := TranslateXrayInbound(raw)
	if err == nil || !strings.Contains(err.Error(), "required TLS") {
		t.Fatalf("expected Hysteria2 TLS validation error, got %v", err)
	}
}

func TestTranslateHysteria2InboundEmptyStreamRequiresTLS(t *testing.T) {
	raw := map[string]any{
		"protocol": "hysteria2",
		"tag":      "hy2-empty-stream",
		"port":     443,
	}
	_, err := TranslateXrayInbound(raw)
	if err == nil || !strings.Contains(err.Error(), "required TLS") {
		t.Fatalf("expected Hysteria2 TLS validation error for empty stream, got %v", err)
	}
}

func TestTranslateHysteria2OutboundSynthesizesTLSAndSkipsInboundOnlyOption(t *testing.T) {
	raw := map[string]any{
		"protocol": "hysteria",
		"tag":      "hy2-out-auto-tls",
		"settings": map[string]any{
			"address": "example.com",
			"port":    443,
		},
		"streamSettings": map[string]any{
			"network": "hysteria",
			"hysteriaSettings": map[string]any{
				"version":                 2,
				"auth":                    "secret",
				"ignore_client_bandwidth": true,
			},
		},
	}

	got, err := TranslateXrayOutbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["ignore_client_bandwidth"]; ok {
		t.Fatalf("inbound-only ignore_client_bandwidth leaked into outbound: %#v", got)
	}
	tls, ok := got["tls"].(map[string]any)
	if !ok || tls["enabled"] != true {
		t.Fatalf("expected synthesized Hysteria2 outbound TLS: %#v", got["tls"])
	}
}

func TestTranslateHysteria2DoesNotMapUDPIdleTimeoutToQUICIdleTimeout(t *testing.T) {
	raw := map[string]any{
		"protocol": "hysteria",
		"tag":      "hy2-udp-timeout",
		"port":     443,
		"streamSettings": map[string]any{
			"network":  "hysteria",
			"security": "tls",
			"hysteriaSettings": map[string]any{
				"version":        2,
				"udpIdleTimeout": 60,
			},
		},
	}

	got, err := TranslateXrayInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got["idle_timeout"]; ok {
		t.Fatalf("udpIdleTimeout must not be mapped to QUIC idle_timeout: %#v", got)
	}
}

func TestTranslateHysteria2GeckoNormalizesPacketSizeAliases(t *testing.T) {
	raw := map[string]any{
		"protocol": "hysteria",
		"tag":      "hy2-gecko",
		"port":     443,
		"streamSettings": map[string]any{
			"network":  "hysteria",
			"security": "tls",
			"hysteriaSettings": map[string]any{
				"version": 2,
				"obfs": map[string]any{
					"type":          "Gecko",
					"password":      "secret",
					"minPacketSize": 600,
					"maxPacketSize": 1100,
				},
			},
		},
	}

	got, err := TranslateXrayInbound(raw)
	if err != nil {
		t.Fatal(err)
	}
	obfs, ok := got["obfs"].(map[string]any)
	if !ok || obfs["type"] != "gecko" || obfs["min_packet_size"] != 600 || obfs["max_packet_size"] != 1100 {
		t.Fatalf("gecko aliases were not normalized: %#v", got["obfs"])
	}
}

func TestTranslateHysteria2RejectsInvalidObfs(t *testing.T) {
	raw := map[string]any{
		"protocol": "hysteria",
		"tag":      "hy2-invalid-obfs",
		"port":     443,
		"settings": map[string]any{
			"clients": []any{map[string]any{"auth": "secret"}},
		},
		"streamSettings": map[string]any{
			"network":  "hysteria",
			"security": "tls",
			"hysteriaSettings": map[string]any{
				"version": 2,
				"obfs": map[string]any{
					"type":     "unknown",
					"password": "secret",
				},
			},
		},
	}

	_, err := TranslateXrayInbound(raw)
	if err == nil || !strings.Contains(err.Error(), "obfs type") {
		t.Fatalf("expected Hysteria2 obfs validation error, got %v", err)
	}
}
