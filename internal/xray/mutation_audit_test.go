package xray

import (
	"slices"
	"testing"
)

// ---------------------------------------------------------------------------
// hot_diff.go mutation audits
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// process.go mutation audits (pure-logic, cross-platform)
// ---------------------------------------------------------------------------

// TestRefreshLocalOnline_GraceBoundaryEmails pins the exact `<` boundary at
// process.go:407: an email idle for EXACTLY graceMs must be aged out (the
// window is half-open, age < grace). A mutated comparison (`<=`) would keep it.
func TestRefreshLocalOnline_GraceBoundaryEmails(t *testing.T) {
	p := newOnlineTestProcess()
	const grace = int64(20000)

	p.RefreshLocalOnline([]string{"edge"}, nil, 0, grace)
	// now-ts == grace exactly: age is not strictly < grace, so it must drop.
	p.RefreshLocalOnline(nil, nil, grace, grace)
	for _, e := range p.GetLocalOnlineClients() {
		if e == "edge" {
			t.Fatalf("email idle exactly graceMs must age out (half-open window), got online %v", p.GetLocalOnlineClients())
		}
	}

	// One millisecond inside the window must still be online.
	p2 := newOnlineTestProcess()
	p2.RefreshLocalOnline([]string{"edge"}, nil, 0, grace)
	p2.RefreshLocalOnline(nil, nil, grace-1, grace)
	if !containsString(p2.GetLocalOnlineClients(), "edge") {
		t.Fatalf("email idle graceMs-1 must still be online, got %v", p2.GetLocalOnlineClients())
	}
}

// TestRefreshLocalOnline_GraceBoundaryInbounds pins the same `<` boundary at
// process.go:423 for inbound tags.
func TestRefreshLocalOnline_GraceBoundaryInbounds(t *testing.T) {
	p := newOnlineTestProcess()
	const grace = int64(20000)

	p.RefreshLocalOnline(nil, []string{"in-edge"}, 0, grace)
	p.RefreshLocalOnline(nil, nil, grace, grace)
	for _, tag := range p.GetLocalActiveInbounds() {
		if tag == "in-edge" {
			t.Fatalf("inbound idle exactly graceMs must age out, got active %v", p.GetLocalActiveInbounds())
		}
	}

	p2 := newOnlineTestProcess()
	p2.RefreshLocalOnline(nil, []string{"in-edge"}, 0, grace)
	p2.RefreshLocalOnline(nil, nil, grace-1, grace)
	if !containsString(p2.GetLocalActiveInbounds(), "in-edge") {
		t.Fatalf("inbound idle graceMs-1 must still be active, got %v", p2.GetLocalActiveInbounds())
	}
}

func containsString(s []string, v string) bool {
	return slices.Contains(s, v)
}

// ---------------------------------------------------------------------------
// process.go mutation audits (require a real child process; re-invoke the
// test binary so they run cross-platform, no signals needed)
// ---------------------------------------------------------------------------
