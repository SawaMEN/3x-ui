package service

import (
	"strings"
	"testing"
)

func TestGatewayPolicyRoutePresentRequiresLocalDefault(t *testing.T) {
	if gatewayPolicyRoutePresent("default via 192.0.2.1 dev eth0\n") {
		t.Fatal("default route must not be treated as the Gateway local route")
	}
	if !gatewayPolicyRoutePresent("local default dev lo scope host\n") {
		t.Fatal("expected Gateway local route to be detected")
	}
}

func TestGatewayNFTRulesScopeInterceptionToLANInterface(t *testing.T) {
	rules := gatewayNFTRules("br-lan", "192.168.50.0/24", "")

	if !strings.Contains(rules, `iifname "br-lan" meta nfproto ipv4 jump tp_pre`) {
		t.Fatal("Gateway prerouting must be scoped to the selected LAN interface")
	}
	if strings.Contains(rules, `iifname "br-*" return`) {
		t.Fatal("selected bridge LAN interfaces must not be blanket-excluded")
	}
	if strings.Contains(rules, "tproxy ip6") || strings.Contains(rules, "meta nfproto { ipv4, ipv6 }") {
		t.Fatal("IPv6 must not be intercepted without matching IPv6 policy routing support")
	}
}

func TestGatewayNFTRulesWANMasquerade(t *testing.T) {
	rules := gatewayNFTRules("eth1", "10.20.30.0/24", "eth0")

	if !strings.Contains(rules, "table ip xui_gateway_nat") {
		t.Fatal("WAN configuration must create the Gateway NAT table")
	}
	if !strings.Contains(rules, `ip saddr 10.20.30.0/24 oifname "eth0" masquerade`) {
		t.Fatal("WAN NAT rule does not match the selected network/interface")
	}
}

func TestSafeGatewayInterfaceName(t *testing.T) {
	for _, name := range []string{"eth0", "enp2s0", "br-lan", "eth0.100", "wg_test"} {
		if !safeGatewayInterfaceName(name) {
			t.Fatalf("expected interface name %q to be accepted", name)
		}
	}
	for _, name := range []string{"", "bad name", `bad"name`, "bad/name", "0123456789abcdef"} {
		if safeGatewayInterfaceName(name) {
			t.Fatalf("expected interface name %q to be rejected", name)
		}
	}
}
