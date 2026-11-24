package antidpi

import (
	"net/netip"
	"testing"
)

func TestSiberianClassification(t *testing.T) {
	ok := TLSAttempt{Stage: "tls", Outcome: "ok"}
	timeout := TLSAttempt{Stage: "tls", Outcome: "timeout"}
	r := SiberianReport{Baseline: []TLSAttempt{ok, ok}, Burst: []TLSAttempt{ok, ok, ok, ok}, Control: &ok}
	if got := classifySiberian(r); got != "no-signal" {
		t.Fatal(got)
	}
	r.Burst[0] = timeout
	if got := classifySiberian(r); got != "asymmetry" {
		t.Fatal(got)
	}
	r.Control = &timeout
	if got := classifySiberian(r); got != "inconclusive" {
		t.Fatal(got)
	}
	r.Control = &ok
	r.Burst[0] = TLSAttempt{Stage: "tcp", Outcome: "timeout"}
	if got := classifySiberian(r); got != "inconclusive" {
		t.Fatal(got)
	}
	r.Baseline[0] = timeout
	if got := classifySiberian(r); got != "inconclusive" {
		t.Fatal(got)
	}
}

func TestSharedAddress(t *testing.T) {
	a := netip.MustParseAddr("1.1.1.1")
	b := netip.MustParseAddr("8.8.8.8")
	if got, err := sharedAddress([]netip.Addr{a,b}, []netip.Addr{a}); err != nil || got != a {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := sharedAddress([]netip.Addr{a,b}, []netip.Addr{a,b}); err == nil {
		t.Fatal("ambiguous IP accepted")
	}
	if _, err := sharedAddress([]netip.Addr{a}, []netip.Addr{b}); err == nil {
		t.Fatal("different IP accepted")
	}
}
