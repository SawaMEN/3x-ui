package sub

import (
	"reflect"
	"testing"
)

func TestStandaloneSubscriptionLinks(t *testing.T) {
	raw := []string{
		"vless://uuid@example.org:443#standard",
		"pingtunnel://eyJ0eXBlIjoiY2xpZW50In0#icmp\nopenflux://v1/aabb#flux",
		"fptn:YWJjZA==",
		"sudoku://example.org#sidecar",
		"tt://example.org#trust",
		"ss://YWJjZA==@example.org:443#standard",
	}
	got := standaloneSubscriptionLinks(raw)
	want := []string{
		"pingtunnel://eyJ0eXBlIjoiY2xpZW50In0#icmp",
		"openflux://v1/aabb#flux",
		"fptn:YWJjZA==",
		"sudoku://example.org#sidecar",
		"tt://example.org#trust",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("standalone links = %#v, want %#v", got, want)
	}
	for _, standard := range []string{"vless://x", "vmess://x", "ss://x", "hysteria2://x"} {
		if isStandaloneSubscriptionLink(standard) {
			t.Fatalf("stock core link classified as standalone: %s", standard)
		}
	}
}
