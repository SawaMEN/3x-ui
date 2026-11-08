package singbox

import (
	"strings"
	"testing"
)

func TestStage5QUICRejectsUnsupportedTLSBeforeRuntime(t *testing.T) {
	for _, tc := range []struct {
		name string
		tls  map[string]any
		ok   bool
	}{
		{"Go TLS", map[string]any{"enabled": true}, true},
		{"uTLS", map[string]any{"enabled": true, "utls": map[string]any{"enabled": true, "fingerprint": "chrome"}}, false},
		{"disabled uTLS", map[string]any{"enabled": true, "utls": map[string]any{"enabled": false}}, true},
		{"disabled TLS", map[string]any{"enabled": false}, false},
		{"TLS12", map[string]any{"enabled": true, "max_version": "1.2"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Outbounds: []map[string]any{{"type": "vless", "tag": "proxy", "server": "127.0.0.1", "server_port": 443, "uuid": "11111111-1111-4111-8111-111111111111", "transport": map[string]any{"type": "quic"}, "tls": tc.tls}}}
			_, err := cfg.Marshal()
			if (err == nil) != tc.ok {
				t.Fatalf("allowed=%v err=%v", tc.ok, err)
			}
			if !tc.ok && !strings.Contains(err.Error(), "QUIC") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
