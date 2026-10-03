package service

import (
	"encoding/json"
	"testing"
)

func TestNormalizeTemplateNativeSingBoxOutbounds(t *testing.T) {
	raw := `{"outbounds":[{"type":"future-proxy","tag":"native","server":"example.com","server_port":443,"custom":true}]}`
	got, err := normalizeTemplateNativeSingBoxOutbounds(raw)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}

	var cfg map[string]any
	if err := json.Unmarshal([]byte(got), &cfg); err != nil {
		t.Fatalf("unmarshal normalized config: %v", err)
	}
	outbounds, _ := cfg["outbounds"].([]any)
	if len(outbounds) != 1 {
		t.Fatalf("outbounds = %d", len(outbounds))
	}
	out, _ := outbounds[0].(map[string]any)
	if got := out["protocol"]; got != "singbox:future-proxy" {
		t.Fatalf("protocol = %v", got)
	}
	if got := out["tag"]; got != "native" {
		t.Fatalf("tag = %v", got)
	}
	settings, _ := out["settings"].(map[string]any)
	if settings["server"] != "example.com" || settings["server_port"] != float64(443) || settings["custom"] != true {
		t.Fatalf("settings not preserved: %#v", settings)
	}
	if _, exists := settings["type"]; exists {
		t.Fatalf("native type leaked into settings: %#v", settings)
	}
}

func TestNormalizeTemplateNativeSingBoxOutboundsLeavesXrayShapeUntouched(t *testing.T) {
	raw := `{"outbounds":[{"protocol":"future-xray-proxy","tag":"future","settings":{"server":"example.com"}}]}`
	got, err := normalizeTemplateNativeSingBoxOutbounds(raw)
	if err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if got != raw {
		t.Fatalf("Xray-shaped outbound was modified:\nwant: %s\n got: %s", raw, got)
	}
}

func TestNormalizeTemplateNativeSingBoxOutboundsRejectsNonArray(t *testing.T) {
	_, err := normalizeTemplateNativeSingBoxOutbounds(`{"outbounds":{"type":"direct"}}`)
	if err == nil {
		t.Fatal("expected error for non-array outbounds")
	}
}
