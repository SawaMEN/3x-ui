package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestSingBoxInboundRequiresUsers(t *testing.T) {
	testCases := []struct {
		protocol model.Protocol
		required bool
	}{
		{model.VLESS, true},
		{model.VMESS, true},
		{model.Trojan, true},
		{model.NaiveProxy, true},
		{model.AnyTLS, true},
		{model.ShadowTLS, false},
		{model.Hysteria, false},
		{model.HTTP, false},
		{model.Mixed, false},
		{model.Shadowsocks, false},
	}

	for _, tc := range testCases {
		if got := singBoxInboundRequiresUsers(tc.protocol); got != tc.required {
			t.Fatalf("protocol %q: got %v, want %v", tc.protocol, got, tc.required)
		}
	}
}
