package singbox

import (
	"reflect"
	"testing"
)

func TestNormalizeNativeShareOptionTypes(t *testing.T) {
	t.Run("hysteria string auth", func(t *testing.T) {
		cfg := map[string]any{"auth": "secret"}
		if err := normalizeNativeShareOptionTypes(cfg, "hysteria", "h1"); err != nil {
			t.Fatal(err)
		}
		if cfg["auth_str"] != "secret" {
			t.Fatalf("missing string authentication: %#v", cfg)
		}
		if _, exists := cfg["auth"]; exists {
			t.Fatalf("invalid string auth retained: %#v", cfg)
		}
	})
	t.Run("hysteria byte auth unchanged", func(t *testing.T) {
		cfg := map[string]any{"auth": []any{float64(1), float64(2)}}
		if err := normalizeNativeShareOptionTypes(cfg, "hysteria", "h1"); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(cfg["auth"], []any{float64(1), float64(2)}) {
			t.Fatalf("byte auth changed: %#v", cfg)
		}
	})
	t.Run("naive JSON headers", func(t *testing.T) {
		cfg := map[string]any{"extra_headers": `{"X-Foo":"bar"}`}
		if err := normalizeNativeShareOptionTypes(cfg, "naive", "n1"); err != nil {
			t.Fatal(err)
		}
		h, ok := cfg["extra_headers"].(map[string]any)
		if !ok || h["X-Foo"] != "bar" {
			t.Fatal(cfg)
		}
	})
	t.Run("naive simple host", func(t *testing.T) {
		cfg := map[string]any{"extra_headers": "example.com"}
		if err := normalizeNativeShareOptionTypes(cfg, "naive", "n1"); err != nil {
			t.Fatal(err)
		}
		h, ok := cfg["extra_headers"].(map[string]any)
		if !ok || h["Host"] != "example.com" {
			t.Fatal(cfg)
		}
	})
	t.Run("naive map preserved", func(t *testing.T) {
		cfg := map[string]any{"extra_headers": map[string]any{"X": "Y"}}
		if err := normalizeNativeShareOptionTypes(cfg, "naive", "n1"); err != nil {
			t.Fatal(err)
		}
		h := cfg["extra_headers"].(map[string]any)
		if h["X"] != "Y" {
			t.Fatal(cfg)
		}
	})
	t.Run("naive rejects invalid headers", func(t *testing.T) {
		cfg := map[string]any{"extra_headers": "{broken"}
		if err := normalizeNativeShareOptionTypes(cfg, "naive", "n1"); err == nil {
			t.Fatal("expected error")
		}
	})
}
