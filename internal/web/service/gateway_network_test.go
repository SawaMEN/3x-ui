package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestGatewayPolicyRuleBoundaries(t *testing.T) {
	for _, tc := range []struct {
		rule          string
		table, policy bool
	}{
		{"32765: from all fwmark 0x40/0xc0 lookup 100\n", true, true},
		{"32765: from all fwmark 0x40/0xc0 table 100 proto static\n", true, true},
		{"32765: from all lookup 100\n", true, false},
		{"32765: from all fwmark 0x40/0xc0 lookup 1000\n", false, false},
		{"32765: from all fwmark 0x40/0xc0 lookup 1001\n", false, false},
	} {
		if gatewayTableInUse(tc.rule) != tc.table || gatewayPolicyRulePresent(tc.rule) != tc.policy {
			t.Fatalf("wrong routing-table ownership for %q", tc.rule)
		}
	}
}

func TestGatewayNFTRulesScopeAndNAT(t *testing.T) {
	rules := gatewayNFTRules("192.168.10.0/24", "eth0", "br-lan")
	for _, fragment := range []string{"meta nfproto != ipv4 return", `iifname != "br-lan" return`, "ip saddr != 192.168.10.0/24 return", "tproxy ip to 127.0.0.1:52345", `oifname "eth0" masquerade`, "meta mark & 0xffffff3f"} {
		if !strings.Contains(rules, fragment) {
			t.Fatalf("missing %q", fragment)
		}
	}
	if strings.Contains(rules, "tproxy ip6") {
		t.Fatal("IPv6 intercepted without an IPv6 listener/route")
	}
	if strings.Contains(rules, `iifname "br-*" return`) {
		t.Fatal("selected LAN bridge is bypassed")
	}
	if strings.Contains(gatewayNFTRules("192.168.10.0/24", "", "eth1"), "masquerade") {
		t.Fatal("NAT enabled without a WAN interface")
	}
}

func TestGatewayRejectsUnsafeInterfaceNames(t *testing.T) {
	for _, name := range []string{"eth0\nWAN_IF=evil", `eth0"`, "$(id)", "eth0;id", "eth0/../all"} {
		if gatewayInterfaceName.MatchString(name) {
			t.Fatalf("unsafe interface name accepted: %q", name)
		}
	}
	for _, name := range []string{"br-lan", "eth0.10", "enp1s0", "wg0"} {
		if !gatewayInterfaceName.MatchString(name) {
			t.Fatalf("valid interface name rejected: %q", name)
		}
	}
}

func TestGatewayCommandHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	started := time.Now()
	if _, err := runGatewayCommand(ctx, "sh", "-c", "exit 0"); err == nil {
		t.Fatal("canceled command succeeded")
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancellation did not stop command promptly")
	}
}
