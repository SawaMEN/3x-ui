package singbox

import (
	"fmt"
	"testing"
)

func BenchmarkStage5TLSNormalization(b *testing.B) {
	for _, withTLS := range []bool{false, true} {
		b.Run(fmt.Sprint("TLS=", withTLS), func(b *testing.B) {
			nodes := make([]map[string]any, 100)
			for i := range nodes {
				nodes[i] = map[string]any{"tag": fmt.Sprint(i), "type": "vless", "users": []any{map[string]any{"uuid": "11111111-1111-4111-8111-111111111111", "name": "alice"}}, "transport": map[string]any{"type": "ws", "path": "/vpn", "headers": map[string]any{"Host": "localhost"}}}
				if withTLS {
					nodes[i]["tls"] = map[string]any{"enabled": true, "cipher_suites": []string{"TLS_RSA_WITH_AES_128_GCM_SHA256", "TLS_AES_128_GCM_SHA256"}}
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				cfg := configJSON{Inbounds: nodes}
				if err := normalizeRuntimeTLSConfig(&cfg); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
