package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestSingBoxInboundRequiresUsers(t *testing.T) {
	tests := []struct {
		name     string
		protocol model.Protocol
		want     bool
	}{
		{name: "vless", protocol: model.VLESS, want: true},
		{name: "vmess", protocol: model.VMESS, want: true},
		{name: "trojan", protocol: model.Trojan, want: true},
		{name: "naive", protocol: model.NaiveProxy, want: true},
		{name: "hysteria", protocol: model.Hysteria, want: true},
		{name: "shadowtls", protocol: model.ShadowTLS, want: true},
		{name: "anytls", protocol: model.AnyTLS, want: true},
		{name: "shadowsocks", protocol: model.Shadowsocks, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := singBoxInboundRequiresUsers(tt.protocol); got != tt.want {
				t.Fatalf("singBoxInboundRequiresUsers(%v) = %v, want %v", tt.protocol, got, tt.want)
			}
		})
	}
}
