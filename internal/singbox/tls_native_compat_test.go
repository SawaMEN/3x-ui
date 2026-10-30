package singbox

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestConfigMarshalNativeTLSCompatibility(t *testing.T) {
	const safe = "TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256"
	const legacy = "TLS_RSA_WITH_AES_128_GCM_SHA256"
	cfg := &Config{
		Inbounds:  []map[string]any{{"type": "hysteria2", "tag": "in-17956-udp", "tls": map[string]any{"enabled": true, "cipher_suites": []any{safe, legacy}}}},
		Outbounds: []map[string]any{{"type": "vless", "tag": "proxy", "server": "example.com", "server_port": 443, "uuid": "11111111-1111-4111-8111-111111111111", "tls": map[string]any{"enabled": true, "cipher_suites": []string{safe, legacy}}, "transport": map[string]any{"type": "xhttp", "mode": "stream-up", "path": "/", "download": map[string]any{"server": "example.com", "server_port": 443, "tls": map[string]any{"enabled": true, "cipher_suites": []any{legacy, safe}}}}}},
		DNS:       map[string]any{"servers": []map[string]any{{"type": "tls", "tag": "remote", "server": "1.1.1.1", "tls": map[string]any{"enabled": true, "cipher_suites": []string{safe, legacy}}}}},
	}
	before, _ := json.Marshal(configJSON(*cfg))
	for range 2 {
		data, err := cfg.Marshal()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), legacy) {
			t.Fatalf("legacy cipher reached final config: %s", data)
		}
		if strings.Count(string(data), safe) != 4 {
			t.Fatalf("lost supported cipher in nested TLS blocks: %s", data)
		}
	}
	after, _ := json.Marshal(configJSON(*cfg))
	if string(before) != string(after) {
		t.Fatal("serialization mutated saved native settings")
	}
}

func TestConfigMarshalRejectsInvalidNativeTLSCipherSuites(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value any
		want  string
	}{
		{"obsolete only", []string{"TLS_RSA_WITH_AES_128_GCM_SHA256"}, "no supported TLS ciphers"},
		{"unknown", []any{"TLS_TYPO"}, "unknown TLS cipher suite"},
		{"malformed", []any{42}, "must be a string"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := &Config{Inbounds: []map[string]any{{"type": "hysteria2", "tag": "in-17956-udp", "tls": map[string]any{"enabled": true, "cipher_suites": tc.value}}}}
			_, err := cfg.Marshal()
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "in-17956-udp") {
				t.Fatalf("error = %v", err)
			}
		})
	}
}

func TestConfigMarshalAcceptsNativeCipherDefaultsAndScalar(t *testing.T) {
	for _, value := range []any{nil, []string{}, []any{}, "TLS_AES_128_GCM_SHA256"} {
		cfg := &Config{Inbounds: []map[string]any{{"type": "hysteria2", "tag": "native", "tls": map[string]any{"enabled": true, "cipher_suites": value}}}}
		if _, err := cfg.Marshal(); err != nil {
			t.Fatalf("%#v: %v", value, err)
		}
	}
}
