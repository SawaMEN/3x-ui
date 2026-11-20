package singbox

import "testing"

// The Xray-compatible editor must preserve fingerprint names supported by
// the SawaMEN native sing-box core rather than silently rewriting versions.
func TestTranslateOutboundTLSVersionedFingerprints(t *testing.T) {
	for _, fingerprint := range []string{
		"chrome",
		"randomizednoalpn",
		"firefox",
		"safari",
	} {
		t.Run(fingerprint, func(t *testing.T) {
			out := map[string]any{"tag": "tls"}
			stream := map[string]any{
				"security": "tls",
				"tlsSettings": map[string]any{
					"fingerprint": fingerprint,
					"serverName":  "example.com",
				},
			}
			if err := translateOutboundTLSCompatibility(out, stream, "tls"); err != nil {
				t.Fatal(err)
			}
			tlsOut, ok := out["tls"].(map[string]any)
			if !ok {
				t.Fatalf("missing sing-box TLS configuration: %#v", out)
			}
			utls, ok := tlsOut["utls"].(map[string]any)
			if !ok || utls["enabled"] != true || utls["fingerprint"] != fingerprint {
				t.Fatalf("fingerprint changed in translation: %#v", tlsOut["utls"])
			}
		})
	}
}

func TestTranslateOutboundTLSMigratesObsoleteFingerprints(t *testing.T) {
	for _, tc := range []struct{ old, current string }{
		{"hellochrome_133", "chrome"},
		{"hellochrome_155", "chrome"},
		{"chrome_psk", "chrome"},
		{"chrome_pq_psk", "chrome"},
		{"hellofirefox_148", "firefox"},
		{"hellosafari_26_3", "safari"},
	} {
		t.Run(tc.old, func(t *testing.T) {
			out := map[string]any{"tag": "legacy-tls"}
			in := map[string]any{
				"security": "tls",
				"tlsSettings": map[string]any{
					"fingerprint": tc.old,
					"serverName":  "example.com",
				},
			}
			if err := translateOutboundTLSCompatibility(out, in, "legacy-tls"); err != nil {
				t.Fatal(err)
			}
			utlsOut := out["tls"].(map[string]any)["utls"].(map[string]any)
			if got := utlsOut["fingerprint"]; got != tc.current {
				t.Fatalf("legacy fingerprint %q migrated to %q, want %q", tc.old, got, tc.current)
			}
		})
	}
}
