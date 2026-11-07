package service

import (
	"net"
	"testing"

	xraynet "github.com/xtls/xray-core/common/net"
)

func TestBuildXrayRouteContext(t *testing.T) {
	tests := []struct {
		name     string
		request  RouteTestRequest
		network  xraynet.Network
		domain   string
		wantIP   net.IP
		wantPort uint32
	}{
		{
			name: "TCP domain",
			request: RouteTestRequest{Domain: "example.org", Port: 443, Network: "tcp", InboundTag: "in", Protocol: "tls"},
			network: xraynet.Network_TCP, domain: "example.org", wantPort: 443,
		},
		{
			name: "UDP IPv4",
			request: RouteTestRequest{IP: "203.0.113.5", Port: 53, Network: "udp"},
			network: xraynet.Network_UDP, wantIP: net.ParseIP("203.0.113.5"), wantPort: 53,
		},
		{
			name: "UDP IPv6",
			request: RouteTestRequest{IP: "2001:db8::3", Port: 443, Network: "udp"},
			network: xraynet.Network_UDP, wantIP: net.ParseIP("2001:db8::3"), wantPort: 443,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildXrayRouteContext(tt.request)
			if err != nil {
				t.Fatal(err)
			}
			if got.Network != tt.network || got.TargetDomain != tt.domain || got.TargetPort != tt.wantPort {
				t.Fatalf("unexpected routing context: %+v", got)
			}
			if tt.wantIP != nil {
				if len(got.TargetIPs) != 1 || !net.IP(got.TargetIPs[0]).Equal(tt.wantIP) {
					t.Fatalf("unexpected destination IP: %v", got.TargetIPs)
				}
			}
		})
	}
}

func TestBuildXrayRouteContextRejectsInvalidInput(t *testing.T) {
	tests := []RouteTestRequest{
		{Domain: "example.org", IP: "127.0.0.1", Network: "tcp"},
		{Domain: "https://example.org", Network: "tcp"},
		{Domain: "example.org", Network: "all"},
		{Domain: "example.org", Network: "tcp", Port: 65536},
		{IP: "not-an-ip", Network: "udp"},
		{Network: "tcp"},
	}
	for _, request := range tests {
		if _, err := buildXrayRouteContext(request); err == nil {
			t.Errorf("accepted invalid input: %+v", request)
		}
	}
}
