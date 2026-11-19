package singbox

import "testing"

// The Xray-compatible editor must preserve fingerprint names supported by
// the SawaMEN native sing-box core rather than silently rewriting versions.
func TestTranslateOutboundTLSVersionedFingerprints(t *testing.T) {
	for _, fingerprint := range []string{
		"chrome",
		"hellochrome_155",
		"hellochrome_133",
		"firefox",
		"hellofirefox_148",
		"safari",
		"hellosafari_26_3",
	} {
		t.Run(fingerprint, func(t *testing.T) {
			out := map[string]any{"tag": "tls"}
			stream := map[string]any{
				"security": "tls",
				"tlsSettings": map[string]any{
					"fingerprint": fingerprint,
					"serverName": "example.com",
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
