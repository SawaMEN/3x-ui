package singbox

import (
	"reflect"
	"strings"
	"testing"
)

func TestTLSCipherCompatibility(t *testing.T) {
	const safe = "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256"
	for _, inbound := range []bool{true, false} {
		for _, tc := range []struct {
			name, suites, wantErr string
			want                  []string
		}{
			{name: "old Firefox preset", suites: safe + ":TLS_RSA_WITH_AES_128_GCM_SHA256:TLS_RSA_WITH_AES_256_GCM_SHA384:TLS_RSA_WITH_AES_128_CBC_SHA:TLS_RSA_WITH_AES_256_CBC_SHA", want: []string{safe, "TLS_RSA_WITH_AES_128_GCM_SHA256", "TLS_RSA_WITH_AES_256_GCM_SHA384", "TLS_RSA_WITH_AES_128_CBC_SHA", "TLS_RSA_WITH_AES_256_CBC_SHA"}},
			{name: "normalize", suites: " " + safe + ", " + safe + " : TLS_AES_128_GCM_SHA256 ", want: []string{safe, "TLS_AES_128_GCM_SHA256"}},
			{name: "unknown", suites: safe + ":TLS_TYPO", wantErr: "unknown TLS cipher suite"},
			{name: "legacy only", suites: "TLS_RSA_WITH_AES_128_GCM_SHA256", want: []string{"TLS_RSA_WITH_AES_128_GCM_SHA256"}},
			{name: "empty", suites: " "},
		} {
			t.Run(tc.name+map[bool]string{true: " inbound", false: " outbound"}[inbound], func(t *testing.T) {
				out := map[string]any{"tag": "tls"}
				stream := map[string]any{"network": "tcp", "security": "tls", "tlsSettings": map[string]any{"cipherSuites": tc.suites}}
				err := translateStream(out, "vless", stream, inbound)
				if err == nil && !inbound {
					err = translateOutboundTLSCompatibility(out, stream, "tls")
				}
				if tc.wantErr != "" {
					if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
						t.Fatalf("error = %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				var want any
				if tc.want != nil {
					want = tc.want
				}
				if got := out["tls"].(map[string]any)["cipher_suites"]; !reflect.DeepEqual(got, want) {
					t.Fatalf("got %#v, want %#v", got, want)
				}
				if stream["tlsSettings"].(map[string]any)["cipherSuites"] != tc.suites {
					t.Fatal("source settings mutated")
				}
			})
		}
	}
}
