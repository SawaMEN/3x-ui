package netsafe

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
)

func TestNormalizeHostDNSLabelBoundaries(t *testing.T) {
	if got, err := NormalizeHost("vpn.example.com."); err != nil || got != "vpn.example.com" {
		t.Fatalf("absolute DNS hostname = %q, %v", got, err)
	}
	if got, err := NormalizeHost(strings.Repeat("a", 64) + ".example"); err == nil {
		t.Fatalf("accepted oversized DNS label %q", got)
	}
	valid := strings.Repeat("a", 63) + ".example"
	if got, err := NormalizeHost(valid); err != nil || got != valid {
		t.Fatalf("maximum DNS label = %q, %v", got, err)
	}
}

func TestGuardedDialPreservesIPv6ZoneWithPrivateOptIn(t *testing.T) {
	previous := defaultDialer
	t.Cleanup(func() { defaultDialer = previous })
	errStop := errors.New("stop before connecting")
	var target string
	defaultDialer = &net.Dialer{ControlContext: func(_ context.Context, _, address string, _ syscall.RawConn) error {
		target = address
		return errStop
	}}
	ctx := ContextWithAllowPrivate(context.Background(), true)
	_, err := SSRFGuardedDialContext(ctx, "tcp", "[fe80::1%1]:443")
	if !errors.Is(err, errStop) {
		t.Fatalf("dial did not reach socket control: %v", err)
	}
	if target != "[fe80::1%1]:443" {
		t.Fatalf("scoped IPv6 dial lost interface: %q", target)
	}
}
