package service

import "testing"

func TestDecodeInboundJSONMap(t *testing.T) {
	got, err := decodeInboundJSONMap(`{"handshake":{"server":"cloudflare.com","serverPort":443},"clients":[]}`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	handshake, ok := got["handshake"].(map[string]any)
	if !ok {
		t.Fatalf("expected handshake object, got %#v", got["handshake"])
	}
	if handshake["server"] != "cloudflare.com" || handshake["serverPort"] != float64(443) {
		t.Fatalf("unexpected handshake: %#v", handshake)
	}
}

func TestDecodeInboundJSONMapRejectsInvalidJSON(t *testing.T) {
	if _, err := decodeInboundJSONMap(`{"handshake":`); err == nil {
		t.Fatal("expected invalid JSON error")
	}
}
