package service

import (
	"encoding/json"
	"testing"
)

func TestValidateRoutingRules(t *testing.T) {
	good := []json.RawMessage{json.RawMessage(`{"type":"field","outboundTag":"direct"}`)}
	got, err := validateRoutingRules(good)
	if err != nil || len(got) != 1 {
		t.Fatalf("valid rules rejected: %v", err)
	}
	if _, err := validateRoutingRules([]json.RawMessage{json.RawMessage(`[]`)}); err == nil {
		t.Fatal("array rule should be rejected")
	}
	if _, err := validateRoutingRules([]json.RawMessage{json.RawMessage(`not-json`)}); err == nil {
		t.Fatal("invalid rule should be rejected")
	}
}
