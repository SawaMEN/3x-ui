package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestNormalizeFirewallRule(t *testing.T) {
	tests := []struct {
		name    string
		rule    FirewallPortRule
		wantErr bool
		proto   string
	}{
		{name: "tcp", rule: FirewallPortRule{Port: 443, Protocol: "TCP"}, proto: "tcp"},
		{name: "both", rule: FirewallPortRule{Port: 8443, Protocol: " both "}, proto: "both"},
		{name: "bad port", rule: FirewallPortRule{Port: 0, Protocol: "tcp"}, wantErr: true},
		{name: "bad protocol", rule: FirewallPortRule{Port: 443, Protocol: "sctp"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeFirewallRule(tt.rule)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Protocol != tt.proto {
				t.Fatalf("protocol = %q, want %q", got.Protocol, tt.proto)
			}
		})
	}
}

func TestInboundFirewallProtocol(t *testing.T) {
	tests := []struct {
		name    string
		inbound model.Inbound
		want    string
	}{
		{name: "wireguard", inbound: model.Inbound{Protocol: model.WireGuard}, want: "udp"},
		{name: "hysteria", inbound: model.Inbound{Protocol: model.Hysteria}, want: "udp"},
		{name: "vless tcp", inbound: model.Inbound{Protocol: model.VLESS, StreamSettings: `{"network":"tcp"}`}, want: "tcp"},
		{name: "vless quic", inbound: model.Inbound{Protocol: model.VLESS, StreamSettings: `{"network":"quic"}`}, want: "udp"},
		{name: "shadowsocks", inbound: model.Inbound{Protocol: model.Shadowsocks}, want: "both"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := inboundFirewallProtocol(tt.inbound); got != tt.want {
				t.Fatalf("protocol = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDedupeFirewallRules(t *testing.T) {
	rules := []FirewallPortRule{
		{Port: 443, Protocol: "tcp", Label: "panel", Source: "panel"},
		{Port: 443, Protocol: "TCP", Label: "duplicate", Source: "inbound"},
		{Port: 53, Protocol: "udp", Label: "dns", Source: "manual"},
	}
	got := dedupeFirewallRules(rules)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Port != 53 || got[1].Port != 443 {
		t.Fatalf("unexpected sort order: %#v", got)
	}
}
