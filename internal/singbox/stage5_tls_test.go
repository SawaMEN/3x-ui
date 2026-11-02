package singbox

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestStage5TLSNormalizationPreservesSharedSubtrees(t *testing.T) {
	sharedTLS := map[string]any{"cipher_suites": []any{" TLS_AES_128_GCM_SHA256 ", "TLS_AES_128_GCM_SHA256"}}
	shared := map[string]any{"tls": sharedTLS}
	nodes := []map[string]any{{"tag": "nested", "download": []any{shared}, "other": []map[string]any{shared}}, {"tag": "no-tls", "nullable": []any(nil)}}
	before, _ := json.Marshal(nodes)
	cfg := configJSON{Inbounds: nodes}
	if err := normalizeRuntimeTLSConfig(&cfg); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(nodes)
	if string(before) != string(after) {
		t.Fatal("normalization mutated shared saved objects")
	}
	normalized, _ := json.Marshal(cfg.Inbounds)
	if strings.Count(string(normalized), "TLS_AES_128_GCM_SHA256") != 2 || strings.Contains(string(normalized), " TLS_") {
		t.Fatalf("nested normalization failed: %s", normalized)
	}
	// A rejected sibling must not leave earlier shared TLS blocks changed.
	cfg.Inbounds = append(nodes, map[string]any{"tls": map[string]any{"cipher_suites": []any{false}}})
	if err := normalizeRuntimeTLSConfig(&cfg); err == nil {
		t.Fatal("malformed sibling was accepted")
	}
	after, _ = json.Marshal(nodes)
	if string(before) != string(after) {
		t.Fatal("failed normalization mutated saved objects")
	}
}

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
