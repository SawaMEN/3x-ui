package dnstt

import (
	"context"
	"errors"
	"net"
	"testing"
)

type fakeDNSLookup struct {
	records []*net.NS
	ips     []net.IPAddr
	nsErr   error
	ipErr   error
}

func (f fakeDNSLookup) LookupNS(_ context.Context, _ string) ([]*net.NS, error) {
	return f.records, f.nsErr
}

func (f fakeDNSLookup) LookupIPAddr(_ context.Context, _ string) ([]net.IPAddr, error) {
	return f.ips, f.ipErr
}

func TestDNSTTDelegationVerification(t *testing.T) {
	config := Config{Domain: "t.example.com", ServerIP: "203.0.113.2", ListenPort: 53, TargetPort: 22}
	cases := []struct {
		name   string
		lookup fakeDNSLookup
		ready  bool
		ns     bool
	}{
		{"ready", fakeDNSLookup{records: []*net.NS{{Host: "ns.example.com."}}, ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.2")}}}, true, true},
		{"wrong host", fakeDNSLookup{records: []*net.NS{{Host: "other.example.com."}}}, false, false},
		{"wrong IPv4", fakeDNSLookup{records: []*net.NS{{Host: "ns.example.com."}}, ips: []net.IPAddr{{IP: net.ParseIP("203.0.113.3")}}}, false, true},
		{"unresolved NS", fakeDNSLookup{nsErr: errors.New("no such host")}, false, false},
		{"unresolved A", fakeDNSLookup{records: []*net.NS{{Host: "ns.example.com."}}, ipErr: errors.New("no records")}, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			status, err := checkDelegation(context.Background(), config, tc.lookup)
			if err != nil {
				t.Fatal(err)
			}
			if status.Ready != tc.ready || status.NSFound != tc.ns {
				t.Fatalf("unexpected delegation state: %+v", status)
			}
		})
	}
}

func TestDNSTTRejectsCollidingNSZone(t *testing.T) {
	for _, domain := range []string{"ns.example.com", "ns.sub.example.com"} {
		config := Defaults()
		config.Domain = domain
		if err := Validate(config); err == nil {
			t.Errorf("accepted zone colliding with name server: %s", domain)
		}
	}
}
