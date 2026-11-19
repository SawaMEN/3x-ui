package service

import (
	"encoding/json"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestPreparePingtunnelMigratesToV3(t *testing.T) {
	inbound := &model.Inbound{
		Protocol: model.Pingtunnel, Port: 12345,
		Settings: `{"key":0,"encrypt":"chacha20","encryptKey":"","connectTimeout":0}`,
	}
	if err := prepareExternalVPN(inbound, ""); err != nil {
		t.Fatal(err)
	}
	if inbound.Port != 0 {
		t.Fatal("Pingtunnel ICMP must not expose a TCP/UDP port")
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(inbound.Settings), &raw); err != nil {
		t.Fatal(err)
	}
	password, _ := raw["password"].(string)
	if len(password) < 8 {
		t.Fatal("missing generated Pingtunnel 3.x password")
	}
	if _, exists := raw["key"]; exists {
		t.Fatal("legacy v2 key persisted")
	}
	if _, exists := raw["encryptKey"]; exists {
		t.Fatal("legacy v2 encryption key persisted")
	}
	if raw["connectTimeout"] != float64(3000) {
		t.Fatalf("unexpected v3 timeout: %v", raw["connectTimeout"])
	}
	before := inbound.Settings
	if err := prepareExternalVPN(inbound, before); err != nil {
		t.Fatal(err)
	}
	var after map[string]any
	if err := json.Unmarshal([]byte(inbound.Settings), &after); err != nil {
		t.Fatal(err)
	}
	if after["password"] != password {
		t.Fatal("re-saving Pingtunnel unexpectedly rotated the shared password")
	}
}

func TestPreparePingtunnelReusesLegacyEncryptionKey(t *testing.T) {
	inbound := &model.Inbound{
		Protocol: model.Pingtunnel,
		Settings: `{"key":1234,"encrypt":"chacha20","encryptKey":""}`,
	}
	previous := `{"key":1234,"encrypt":"chacha20","encryptKey":"my-old-long-secret"}`
	if err := prepareExternalVPN(inbound, previous); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(inbound.Settings), &raw); err != nil {
		t.Fatal(err)
	}
	if raw["password"] != "my-old-long-secret" {
		t.Fatalf("legacy secret should migrate to the new password field, got %#v", raw["password"])
	}
}
