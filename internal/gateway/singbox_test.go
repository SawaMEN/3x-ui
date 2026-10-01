package gateway

import "testing"

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
		if itemTag(inbound) == inboundTag {
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

func TestRemoveSingBoxGatewayConfigPreservesOtherInbounds(t *testing.T) {
	cfg := map[string]any{
		"inbounds": []any{
			map[string]any{"type": "mixed", "tag": "existing"},
			singBoxGatewayInbound(),
		},
	}

	changed, err := removeTaggedItem(cfg, "inbounds", inboundTag)
	if err != nil {
		t.Fatalf("removeTaggedItem() error = %v", err)
	}
	if !changed {
		t.Fatal("removeTaggedItem() changed = false, want true")
	}

	inbounds := cfg["inbounds"].([]any)
	if len(inbounds) != 1 || itemTag(inbounds[0]) != "existing" {
		t.Fatalf("unrelated inbounds changed: %#v", inbounds)
	}
}
