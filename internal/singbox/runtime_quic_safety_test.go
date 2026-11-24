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
