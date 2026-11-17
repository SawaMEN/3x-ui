package service

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestRenewalFirewallRulesCurrentProcessAndExpiry(t *testing.T) {
	now := time.Now()
	leases := []firewallRenewalLease{
		{ID: "valid", Process: firewallRenewalProcess, Ports: []int{80, 443}, ExpiresAt: now.Add(time.Minute).Unix()},
		{ID: "overlap", Process: firewallRenewalProcess, Ports: []int{80}, ExpiresAt: now.Add(time.Minute).Unix()},
		{ID: "crashed", Process: "old-process", Ports: []int{443}, ExpiresAt: now.Add(time.Hour).Unix()},
		{ID: "expired", Process: firewallRenewalProcess, Ports: []int{80}, ExpiresAt: now.Add(-time.Minute).Unix()},
		{ID: "invalid", Process: firewallRenewalProcess, Ports: []int{65536}, ExpiresAt: now.Add(time.Minute).Unix()},
	}
	got := []int{}
	for _, rule := range firewallRenewalRules(leases, now) {
		got = append(got, rule.Port)
		if rule.Source != "certificate-renewal" || rule.Protocol != "tcp" {
			t.Fatalf("unexpected renewal rule: %+v", rule)
		}
	}
	if !reflect.DeepEqual(got, []int{80, 443}) {
		t.Fatalf("ports = %v", got)
	}
	if validRenewalPorts([]int{0}) || validRenewalPorts([]int{65536}) || !validRenewalPorts([]int{8080, 8443}) || !validRenewalPorts(nil) {
		t.Fatal("renewal ports must be restricted")
	}
}

func TestDNSRenewalDoesNotTouchFirewall(t *testing.T) {
	called := false
	err := WithRenewalFirewall(context.Background(), "dns-01", nil, func(context.Context) error {
		called = true
		return nil
	})
	if err != nil || !called {
		t.Fatalf("DNS-only renewal: called=%v err=%v", called, err)
	}
}
