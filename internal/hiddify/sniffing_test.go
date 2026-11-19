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

func TestSniffingWithoutInactiveFakeDNS(t *testing.T) {
	for _, tc := range []struct {
		name     string
		dest     any
		want     []string
		override bool
	}{
		{name: "default sniffers", dest: []any{"http", "tls", "quic", "fakedns"}, want: []string{"http", "tls", "quic"}, override: true},
		{name: "https alias and FakeDNS", dest: []string{"https", "fakedns"}, want: []string{"tls"}, override: true},
		{name: "FakeDNS only", dest: []any{"fakedns"}},
		{name: "fakeDNS and route-only sniff", dest: []any{"fakedns", "http"}, want: []string{"http"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := map[string]any{"tag": "in-54732-tcp", "sniffing": map[string]any{
				"enabled": true, "routeOnly": !tc.override, "destOverride": tc.dest,
			}}
			before, _ := json.Marshal(raw)
			rule, err := TranslateSniffingRuleWithoutFakeDNS(raw)
			if err != nil {
				t.Fatal(err)
			}
			after, _ := json.Marshal(raw)
			if string(before) != string(after) {
				t.Fatal("source inbound sniffing changed")
			}
			if len(tc.want) == 0 {
				if rule != nil {
					t.Fatalf("inactive FakeDNS created an unfiltered sniff action: %v", rule)
				}
			} else {
				if rule == nil || rule["action"] != "sniff" {
					t.Fatalf("missing sniff action: %v", rule)
				}
				got, ok := rule["sniffer"].([]string)
				if !ok || strings.Join(got, ",") != strings.Join(tc.want, ",") {
					t.Fatalf("sniffers changed: got %v, want %v", rule["sniffer"], tc.want)
				}
				if (rule["override_destination"] == true) != tc.override {
					t.Fatalf("destination override policy changed: %v", rule)
				}
			}
			if _, err := TranslateSniffingRule(raw); err == nil || !strings.Contains(err.Error(), "FakeDNS") {
				t.Fatalf("strict translator silently accepted FakeDNS: %v", err)
			}
		})
	}
}

func TestSniffingWithoutInactiveFakeDNSStillRejectsOtherUnsupportedFields(t *testing.T) {
	raw := map[string]any{"tag": "in-54732-tcp", "sniffing": map[string]any{
		"enabled": true, "destOverride": []any{"fakedns", "http"}, "metadataOnly": true,
	}}
	if _, err := TranslateSniffingRuleWithoutFakeDNS(raw); err == nil || !strings.Contains(err.Error(), "metadataOnly") {
		t.Fatalf("metadataOnly protection bypassed: %v", err)
	}
}
