package hiddify

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestSniffingPreservesDestinationPolicyAndSource(t *testing.T) {
	for _, routeOnly := range []bool{false, true} {
		raw := map[string]any{"tag": "vpn", "sniffing": map[string]any{"enabled": true, "routeOnly": routeOnly, "destOverride": []any{"http", "https"}}}
		before, _ := json.Marshal(raw)
		got, err := TranslateSniffingRule(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got["override_destination"] == true && routeOnly || got["override_destination"] != true && !routeOnly {
			t.Fatalf("destination policy changed: %v", got)
		}
		after, _ := json.Marshal(raw)
		if string(before) != string(after) {
			t.Fatal("sniffing input changed")
		}
		if !routeOnly {
			if _, err := singbox.TranslateXraySniffingRule(raw); err == nil {
				t.Fatal("legacy sing-box extension leaked")
			}
		}
	}
	for _, key := range []string{"domainsExcluded", "metadataOnly"} {
		sniff := map[string]any{"enabled": true, "destOverride": []string{"http"}, key: true}
		if key == "domainsExcluded" {
			sniff[key] = []string{"example.com"}
		}
		if _, err := TranslateSniffingRule(map[string]any{"tag": "vpn", "sniffing": sniff}); err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("unsupported protection lost: %v", err)
		}
	}
}
