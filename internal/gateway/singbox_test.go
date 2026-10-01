package gateway

import (
	"strings"
	"testing"
)

func TestApplySingBoxGatewayConfig(t *testing.T) {
	cfg := map[string]any{
		"inbounds": []any{
			map[string]any{"type": "mixed", "tag": "existing", "listen_port": float64(1080)},
		},
	}

	if err := applySingBoxGatewayConfig(cfg); err != nil {
		t.Fatalf("applySingBoxGatewayConfig() error = %v", err)
	}

	inbounds, ok := cfg["inbounds"].([]any)
	if !ok {
		t.Fatalf("inbounds type = %T, want []any", cfg["inbounds"])
	}
	if len(inbounds) != 2 {
		t.Fatalf("inbound count = %d, want 2", len(inbounds))
	}
	if itemTag(inbounds[0]) != "existing" {
		t.Fatalf("existing inbound was not preserved: %#v", inbounds)
	}

	inbound, ok := inbounds[1].(map[string]any)
	if !ok {
		t.Fatalf("gateway inbound type = %T, want map[string]any", inbounds[1])
	}
	if inbound["type"] != "tproxy" {
		t.Fatalf("gateway inbound type = %#v, want tproxy", inbound["type"])
	}
	if inbound["tag"] != inboundTag {
		t.Fatalf("gateway inbound tag = %#v, want %q", inbound["tag"], inboundTag)
	}
	if inbound["listen"] != "0.0.0.0" {
		t.Fatalf("gateway listen = %#v, want 0.0.0.0", inbound["listen"])
	}
	if inbound["listen_port"] != inboundPort {
		t.Fatalf("gateway listen_port = %#v, want %d", inbound["listen_port"], inboundPort)
	}
	if _, exists := inbound["network"]; exists {
		t.Fatalf("gateway network = %#v, want omitted for TCP+UDP", inbound["network"])
	}
}

func TestApplySingBoxGatewayConfigIsIdempotent(t *testing.T) {
	cfg := map[string]any{
		"inbounds": []any{
			map[string]any{"type": "mixed", "tag": "existing"},
		},
	}

	if err := applySingBoxGatewayConfig(cfg); err != nil {
		t.Fatalf("first applySingBoxGatewayConfig() error = %v", err)
	}
	if err := applySingBoxGatewayConfig(cfg); err != nil {
		t.Fatalf("second applySingBoxGatewayConfig() error = %v", err)
	}

	inbounds := cfg["inbounds"].([]any)
	gatewayCount := 0
	for _, inbound := range inbounds {
		if isSingBoxGatewayInbound(inbound) {
			gatewayCount++
		}
	}
	if gatewayCount != 1 {
		t.Fatalf("gateway inbound count = %d, want 1", gatewayCount)
	}
	if len(inbounds) != 2 {
		t.Fatalf("inbound count = %d, want existing + gateway", len(inbounds))
	}
}

func TestApplySingBoxGatewayConfigRejectsTagCollision(t *testing.T) {
	conflicting := map[string]any{
		"type":        "mixed",
		"tag":         inboundTag,
		"listen":      "127.0.0.1",
		"listen_port": float64(1080),
	}
	cfg := map[string]any{"inbounds": []any{conflicting}}

	err := applySingBoxGatewayConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "tag") {
		t.Fatalf("applySingBoxGatewayConfig() error = %v, want tag collision", err)
	}

	inbounds := cfg["inbounds"].([]any)
	if len(inbounds) != 1 || itemTag(inbounds[0]) != inboundTag {
		t.Fatalf("conflicting inbound was modified: %#v", inbounds)
	}
	inbound := inbounds[0].(map[string]any)
	if inbound["type"] != "mixed" || inbound["listen_port"] != float64(1080) {
		t.Fatalf("conflicting inbound contents changed: %#v", inbound)
	}
}

func TestApplySingBoxGatewayConfigRejectsPortCollision(t *testing.T) {
	conflicting := map[string]any{
		"type":        "mixed",
		"tag":         "existing",
		"listen":      "127.0.0.1",
		"listen_port": float64(inboundPort),
	}
	cfg := map[string]any{"inbounds": []any{conflicting}}

	err := applySingBoxGatewayConfig(cfg)
	if err == nil || !strings.Contains(err.Error(), "port") {
		t.Fatalf("applySingBoxGatewayConfig() error = %v, want port collision", err)
	}

	inbounds := cfg["inbounds"].([]any)
	if len(inbounds) != 1 || itemTag(inbounds[0]) != "existing" {
		t.Fatalf("conflicting inbound was modified: %#v", inbounds)
	}
	inbound := inbounds[0].(map[string]any)
	if inbound["type"] != "mixed" || inbound["listen_port"] != float64(inboundPort) {
		t.Fatalf("conflicting inbound contents changed: %#v", inbound)
	}
}

func TestSingBoxGatewayDetectionRequiresOwnedShape(t *testing.T) {
	cfg := map[string]any{
		"inbounds": []any{
			map[string]any{
				"type":        "mixed",
				"tag":         inboundTag,
				"listen":      "0.0.0.0",
				"listen_port": float64(inboundPort),
			},
		},
	}
	if hasSingBoxGatewayInbound(cfg) {
		t.Fatal("hasSingBoxGatewayInbound() = true for non-Gateway inbound")
	}
}

func TestRemoveSingBoxGatewayConfigPreservesOtherInbounds(t *testing.T) {
	cfg := map[string]any{
		"inbounds": []any{
			map[string]any{"type": "mixed", "tag": "existing"},
			singBoxGatewayInbound(),
		},
	}

	changed, err := removeSingBoxGatewayConfig(cfg)
	if err != nil {
		t.Fatalf("removeSingBoxGatewayConfig() error = %v", err)
	}
	if !changed {
		t.Fatal("removeSingBoxGatewayConfig() changed = false, want true")
	}

	inbounds := cfg["inbounds"].([]any)
	if len(inbounds) != 1 || itemTag(inbounds[0]) != "existing" {
		t.Fatalf("unrelated inbounds changed: %#v", inbounds)
	}
}

func TestRemoveSingBoxGatewayConfigPreservesTagCollision(t *testing.T) {
	conflicting := map[string]any{
		"type":        "mixed",
		"tag":         inboundTag,
		"listen":      "0.0.0.0",
		"listen_port": float64(1080),
	}
	cfg := map[string]any{"inbounds": []any{conflicting}}

	changed, err := removeSingBoxGatewayConfig(cfg)
	if err != nil {
		t.Fatalf("removeSingBoxGatewayConfig() error = %v", err)
	}
	if changed {
		t.Fatal("removeSingBoxGatewayConfig() changed = true for user-owned tag collision")
	}
	inbounds := cfg["inbounds"].([]any)
	if len(inbounds) != 1 || itemTag(inbounds[0]) != inboundTag {
		t.Fatalf("conflicting inbound was modified: %#v", inbounds)
	}
	inbound := inbounds[0].(map[string]any)
	if inbound["type"] != "mixed" || inbound["listen_port"] != float64(1080) {
		t.Fatalf("conflicting inbound contents changed: %#v", inbound)
	}
}
