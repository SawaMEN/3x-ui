package antidpi

import (
	"net/netip"
	"testing"
)

func TestPublicIP(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.0.0.1", "169.254.169.254", "100.64.0.1", "::1", "fc00::1", "::ffff:127.0.0.1", "192.0.2.1", "64:ff9b::a00:1"} {
		if publicIP(netip.MustParseAddr(address)) {
			t.Errorf("accepted %s", address)
		}
	}
	for _, address := range []string{"1.1.1.1", "2606:4700:4700::1111"} {
		if !publicIP(netip.MustParseAddr(address)) {
			t.Errorf("rejected %s", address)
		}
	}
}

func TestValidHost(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "https://example.com", "example.com:443", "a..com", "-a.com", "a-.com"} {
		if validHost(host) {
			t.Errorf("accepted %q", host)
		}
	}
	if !validHost("vpn.example.com") {
		t.Fatal("valid hostname rejected")
	}
}
