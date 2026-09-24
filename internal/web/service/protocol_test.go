package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestIsSingBoxOnlyInboundProtocol(t *testing.T) {
	for _, protocol := range []model.Protocol{
		model.NaiveProxy,
		model.AnyTLS,
		model.ShadowTLS,
	} {
		if !isSingBoxOnlyInboundProtocol(protocol) {
			t.Fatalf("expected %q to require sing-box", protocol)
		}
	}

	for _, protocol := range []model.Protocol{
		model.VLESS,
		model.VMESS,
		model.Trojan,
		model.Shadowsocks,
		model.Hysteria,
	} {
		if isSingBoxOnlyInboundProtocol(protocol) {
			t.Fatalf("expected %q to remain available without the sing-box-only guard", protocol)
		}
	}
}

func TestInboundRuntimeProtocolError(t *testing.T) {
	if err := inboundRuntimeProtocolError(model.AnyTLS); err == nil || err.Error() != "AnyTLS requires sing-box as the selected core" {
		t.Fatalf("unexpected AnyTLS error: %v", err)
	}
}
