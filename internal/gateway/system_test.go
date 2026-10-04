package gateway

import (
	"strings"
	"testing"
)

func TestParseGatewayStateCompatibleWithShell(t *testing.T) {
	state, err := parseGatewayState([]byte(`IP_FORWARD_OLD=0
LAN_IF=eth0
LAN_IP=192.168.50.1
LAN_PREFIX=24
LAN_NETWORK=192.168.50.0/24
WAN_IF=eth1
`))
	if err != nil {
		t.Fatalf("parseGatewayState() error = %v", err)
	}
	if !state.Configured {
		t.Fatal("expected state to be configured")
	}
	if state.OldIPForward != "0" {
		t.Fatalf("OldIPForward = %q, want 0", state.OldIPForward)
	}
	if state.Config.LANInterface != "eth0" || state.Config.LANIP != "192.168.50.1" || state.Config.LANPrefix != 24 {
		t.Fatalf("unexpected LAN config: %+v", state.Config)
	}
	if state.Config.WANInterface != "eth1" {
		t.Fatalf("WANInterface = %q, want eth1", state.Config.WANInterface)
	}
	if state.LANNetwork != "192.168.50.0/24" {
		t.Fatalf("LANNetwork = %q", state.LANNetwork)
	}
}

func TestParseGatewayStateRejectsUnsafeOrIncompleteState(t *testing.T) {
	tests := []string{
		"IP_FORWARD_OLD=2\nLAN_IF=eth0\nLAN_IP=192.168.1.1\nLAN_PREFIX=24\nLAN_NETWORK=192.168.1.0/24\nWAN_IF=\n",
		"IP_FORWARD_OLD=0\nLAN_IF=eth0\nLAN_IP=192.168.1.1\nLAN_PREFIX=invalid\nLAN_NETWORK=192.168.1.0/24\nWAN_IF=\n",
		"IP_FORWARD_OLD=0\nLAN_IF=\nLAN_IP=192.168.1.1\nLAN_PREFIX=24\nLAN_NETWORK=192.168.1.0/24\nWAN_IF=\n",
		"not-an-assignment\n",
	}
	for _, input := range tests {
		if _, err := parseGatewayState([]byte(input)); err == nil {
			t.Fatalf("parseGatewayState(%q) unexpectedly succeeded", input)
		}
	}
}

func TestGatewayNFTRulesContainTPROXYAndLANNetwork(t *testing.T) {
	rules := nftGatewayRules("192.168.50.0/24")
	for _, want := range []string{
		"table inet xui_gateway",
		"192.168.50.0/24",
		"tproxy ip to 127.0.0.1:52345",
		"meta mark & 0x000000c0 == 0x00000040",
	} {
		if !strings.Contains(rules, want) {
			t.Fatalf("nft rules do not contain %q", want)
		}
	}
}

func TestGatewayNATRulesUseValidatedValues(t *testing.T) {
	rules := nftNATRules("10.20.30.0/24", "eth1")
	if !strings.Contains(rules, "ip saddr 10.20.30.0/24 oifname \"eth1\" masquerade") {
		t.Fatalf("unexpected NAT rules: %s", rules)
	}
}
