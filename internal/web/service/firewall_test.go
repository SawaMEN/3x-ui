package service

import (
	"reflect"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestFirewallProtocolsForInbound(t *testing.T) {
	tests := []struct {
		name     string
		protocol model.Protocol
		network  string
		want     []string
	}{
		{name: "vless tcp", protocol: model.VLESS, network: "tcp", want: []string{"tcp"}},
		{name: "vless websocket", protocol: model.VLESS, network: "ws", want: []string{"tcp"}},
		{name: "vless kcp", protocol: model.VLESS, network: "kcp", want: []string{"udp"}},
		{name: "mixed", protocol: model.Mixed, network: "tcp", want: []string{"tcp", "udp"}},
		{name: "wireguard", protocol: model.WireGuard, network: "tcp", want: []string{"udp"}},
		{name: "hysteria", protocol: model.Hysteria, network: "", want: []string{"udp"}},
		{name: "tuic", protocol: model.TUIC, network: "", want: []string{"udp"}},
		{name: "combined", protocol: model.VLESS, network: "tcp,udp", want: []string{"tcp", "udp"}},
		{name: "unknown defaults tcp", protocol: model.VLESS, network: "", want: []string{"tcp"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := firewallProtocolsForInbound(tt.protocol, tt.network)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("firewallProtocolsForInbound(%q, %q) = %#v, want %#v", tt.protocol, tt.network, got, tt.want)
			}
		})
	}
}

func TestNormalizeFirewallProtocols(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{input: "tcp", want: []string{"tcp"}},
		{input: "UDP", want: []string{"udp"}},
		{input: "both", want: []string{"tcp", "udp"}},
		{input: "udp,tcp", want: []string{"tcp", "udp"}},
	}
	for _, tt := range tests {
		got, err := normalizeFirewallProtocols(tt.input)
		if err != nil {
			t.Fatalf("normalizeFirewallProtocols(%q): %v", tt.input, err)
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Fatalf("normalizeFirewallProtocols(%q) = %#v, want %#v", tt.input, got, tt.want)
		}
	}
	if _, err := normalizeFirewallProtocols("icmp"); err == nil {
		t.Fatal("expected invalid protocol error")
	}
}

func TestValidFirewallSpec(t *testing.T) {
	for _, spec := range []string{"22/tcp", "443/TCP", "65535/udp"} {
		if !validFirewallSpec(spec) {
			t.Fatalf("expected %q to be valid", spec)
		}
	}
	for _, spec := range []string{"0/tcp", "65536/tcp", "443/sctp", "443", "abc/tcp"} {
		if validFirewallSpec(spec) {
			t.Fatalf("expected %q to be invalid", spec)
		}
	}
}
