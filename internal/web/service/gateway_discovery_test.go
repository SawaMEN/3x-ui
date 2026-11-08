package service

import (
	"net"
	"testing"
)

func TestGatewayDefaultRouteInterface(t *testing.T) {
	for _, tc := range []struct {
		routes, want string
	}{
		{"default via 203.0.113.1 dev eth0 proto dhcp metric 100\n", "eth0"},
		{"default dev wwan0 scope link\n", "wwan0"},
		{"default via 10.0.0.1 dev eth0\ndefault via 10.0.1.1 dev eth1\n", ""},
		{"192.168.0.0/24 dev eth1\n", ""},
		{"default via 10.0.0.1 dev eth0;reboot\n", ""},
	} {
		if got := gatewayDefaultRouteInterface(tc.routes); got != tc.want {
			t.Fatalf("route %q: got %q want %q", tc.routes, got, tc.want)
		}
	}
}

func TestGatewayDiscoveryFiltersAddresses(t *testing.T) {
	iface := net.Interface{Name: "br-lan", Flags: net.FlagUp}
	info := gatewayInterfaceFromAddresses(iface, []net.Addr{
		&net.IPNet{IP: net.ParseIP("192.168.20.1"), Mask: net.CIDRMask(24, 32)},
		&net.IPNet{IP: net.ParseIP("169.254.1.1"), Mask: net.CIDRMask(16, 32)},
		&net.IPNet{IP: net.ParseIP("2001:db8::1"), Mask: net.CIDRMask(64, 128)},
	}, "eth0")
	if len(info.Addresses) != 1 || !info.Addresses[0].Private || info.Addresses[0].IP != "192.168.20.1" || info.Addresses[0].Prefix != 24 {
		t.Fatalf("address filtering: %#v", info)
	}
}

func TestGatewaySuggestionsAreConservative(t *testing.T) {
	lan := GatewayInterfaceInfo{Name: "br-lan", Addresses: []GatewayInterfaceAddress{{IP: "192.168.30.1", Prefix: 24, Private: true}}}
	wan := GatewayInterfaceInfo{Name: "eth0", DefaultRoute: true, Addresses: []GatewayInterfaceAddress{{IP: "10.0.0.3", Prefix: 24, Private: true}}}
	suggested, _ := gatewaySuggestNetwork([]GatewayInterfaceInfo{lan, wan}, "eth0")
	if suggested == nil || suggested.LANInterface != "br-lan" || suggested.LANIP != "192.168.30.1" || suggested.LANPrefix != 24 || suggested.WANInterface != "eth0" {
		t.Fatalf("expected dedicated private LAN suggestion; got %#v", suggested)
	}
	for _, tc := range []struct {
		name       string
		interfaces []GatewayInterfaceInfo
		wan        string
	}{
		{"unknown-route", []GatewayInterfaceInfo{lan}, ""},
		{"default-route-only", []GatewayInterfaceInfo{wan}, "eth0"},
		{"ambiguous-lan", []GatewayInterfaceInfo{lan, {Name: "eth1", Addresses: []GatewayInterfaceAddress{{IP: "10.10.0.1", Prefix: 24, Private: true}}}, wan}, "eth0"},
		{"public-lan", []GatewayInterfaceInfo{{Name: "eth1", Addresses: []GatewayInterfaceAddress{{IP: "198.51.100.22", Prefix: 24}}}, wan}, "eth0"},
		{"host-only", []GatewayInterfaceInfo{{Name: "br-lan", Addresses: []GatewayInterfaceAddress{{IP: "192.168.0.1", Prefix: 32, Private: true}}}, wan}, "eth0"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, _ := gatewaySuggestNetwork(tc.interfaces, tc.wan); got != nil {
				t.Fatalf("unsafe automatic suggestion: %#v", got)
			}
		})
	}
}
