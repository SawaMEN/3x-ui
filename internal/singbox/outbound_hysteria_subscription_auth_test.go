package singbox

import (
	"strings"
	"testing"
)

// URI decoders may wrap a Hysteria v1 secret in a one-element string
// array. An unnormalized array reaches the kernel as invalid auth[0].
func TestHysteriaShareAuthenticationShapes(t *testing.T) {
	for _, tc := range []struct {
		name string
		value any
		want string
	}{
		{"string", "plain-password", "plain-password"},
		{"json-string-array", []any{"plain-password"}, "plain-password"},
		{"go-string-array", []string{"plain-password"}, "plain-password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]any{"auth": tc.value}
			if err := normalizeNativeShareOptionTypes(raw, "hysteria", "node"); err != nil {
				t.Fatal(err)
			}
			if _, exists := raw["auth"]; exists || raw["auth_str"] != tc.want {
				t.Fatalf("invalid normalized auth: %v", raw)
			}
		})
	}
}

func TestHysteriaRuntimeAuthenticationNormalization(t *testing.T) {
	rows, err := normalizeOutboundsForRuntime([]map[string]any{{
		"type": "hysteria", "tag": "node",
		"server": "example.test", "server_port": 443,
		"auth": []any{"plain-password"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := rows[0]["auth"]; exists || rows[0]["auth_str"] != "plain-password" {
		t.Fatalf("bad runtime authentication: %v", rows[0])
	}
}

func TestHysteriaAuthRejectsAmbiguousStringList(t *testing.T) {
	row := map[string]any{"auth": []any{"first", "second"}}
	if err := normalizeHysteriaShareAuth(row, "node"); err == nil || !strings.Contains(err.Error(), "single authentication") {
		t.Fatalf("accepted ambiguous authentication: %v", err)
	}
}

func TestHysteriaAuthPreservesNativeBytesAndExplicitAuthString(t *testing.T) {
	bytes := map[string]any{"auth": []any{float64(1), float64(2)}}
	if err := normalizeHysteriaShareAuth(bytes, "node"); err != nil || bytes["auth"] == nil {
		t.Fatalf("native auth byte array must remain intact: %v, %v", bytes, err)
	}
	explicit := map[string]any{"auth": []any{"legacy"}, "auth_str": "configured"}
	if err := normalizeHysteriaShareAuth(explicit, "node"); err != nil || explicit["auth_str"] != "configured" {
		t.Fatalf("explicit auth_str was overwritten: %v, %v", explicit, err)
	}
}
