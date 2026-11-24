package singbox

import (
	"strings"
	"testing"
)

func TestRuntimeQUICRejectsIncompatibleTLS(t *testing.T) {
	for _, protocol := range []string{"hysteria", "hysteria2", "tuic", "vless"} {
		for _, tls := range []map[string]any{
			{"enabled": false},
			{"enabled": true, "max_version": "1.2"},
			{"enabled": true, "utls": map[string]any{"enabled": true}},
			{"enabled": true, "reality": map[string]any{"enabled": true}},
		} {
			cfg := &Config{Inbounds: []map[string]any{{"type": protocol, "tag": "test", "transport": map[string]any{"type": "quic"}, "tls": tls}}}
			if _, err := cfg.Marshal(); err == nil || !strings.Contains(err.Error(), "QUIC") {
				t.Fatalf("%s: expected QUIC validation error, got %v", protocol, err)
			}
		}
	}
}

func TestRuntimeNullOutboundReturnsError(t *testing.T) {
	cfg := &Config{Outbounds: []map[string]any{nil}}
	if _, err := cfg.Marshal(); err == nil {
		t.Fatal("null outbound accepted")
	}
}

func TestRuntimeQUICReportsMissingServerBeforeTLS(t *testing.T) {
	for _, protocol := range []string{"hysteria", "hysteria2", "tuic"} {
		outbound := map[string]any{"type": protocol, "tag": "bad"}
		if err := validateSingBoxOutbound(outbound); err == nil || !strings.Contains(err.Error(), "requires a server") {
			t.Fatalf("%s: expected missing server error, got %v", protocol, err)
		}
	}
}

func TestRuntimeQUICOutboundStillRejectsIncompatibleTLS(t *testing.T) {
	for _, protocol := range []string{"hysteria", "hysteria2", "tuic"} {
		outbound := map[string]any{
			"type": protocol, "tag": "bad-tls", "server": "example.com", "server_port": 443,
			"uuid": "2a547fe4-5b0f-4a84-845c-131464875bf8", "password": "secret",
			"up_mbps": 10, "down_mbps": 10,
			"tls": map[string]any{"enabled": true, "max_version": "1.2"},
		}
		if err := validateSingBoxOutbound(outbound); err == nil || !strings.Contains(err.Error(), "QUIC") {
			t.Fatalf("%s: expected QUIC error, got %v", protocol, err)
		}
	}
}
