package sidecartraffic

import (
	"slices"
	"testing"
)

func TestOwnerIdentityChangesRequireOnlyAffectedCounters(t *testing.T) {
	before := &Owner{source: "mieru", dir: "one", inboundID: 1, clients: map[string]int{"changed": 11, "stable": 12}}
	after := &Owner{source: "mieru", dir: "one", inboundID: 1, clients: map[string]int{"stable": 12, "changed": 11}}
	if !before.SameIdentities(after) || len(before.ChangedClients(after)) != 0 {
		t.Fatal("equivalent owner allocation caused a handoff")
	}
	after.clients["changed"] = 13
	if before.SameIdentities(after) {
		t.Fatal("new row ID went unnoticed")
	}
	if got := before.ChangedClients(after); len(got) != 1 || got[0] != "changed" {
		t.Fatalf("unrelated counters required: %v", got)
	}
	moved := *after
	moved.dir = "two"
	got := after.ChangedClients(&moved)
	slices.Sort(got)
	if !slices.Equal(got, []string{"changed", "stable"}) {
		t.Fatalf("namespace handoff omitted counters: %v", got)
	}
	routed := *after
	routed.routed = true
	if after.SameIdentities(&routed) {
		t.Fatal("routed accounting change was ignored")
	}
	var standalone *Owner
	if !standalone.SameIdentities(nil) || len(standalone.ChangedClients(nil)) != 0 {
		t.Fatal("nil standalone owner changed")
	}
	got = standalone.ChangedClients(after)
	slices.Sort(got)
	if !slices.Equal(got, []string{"changed", "stable"}) {
		t.Fatalf("new accounting owner has no baseline boundary: %v", got)
	}
	if before.clients["changed"] != 11 {
		t.Fatal("identity comparison mutated original owner")
	}
}
