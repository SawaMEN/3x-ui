package service

import (
	"reflect"
	"testing"
)

func TestResolveBundleInboundIDs(t *testing.T) {
	ids, missing := resolveBundleInboundIDs(
		map[string]int{"in-a": 4, "in-b": 2},
		[]string{" in-a ", "missing", "in-b", "in-a", "missing"},
	)
	if !reflect.DeepEqual(ids, []int{2, 4}) {
		t.Fatalf("ids = %#v, want [2 4]", ids)
	}
	if !reflect.DeepEqual(missing, []string{"missing"}) {
		t.Fatalf("missing = %#v, want [missing]", missing)
	}
}

func TestNormalizeGroupIDs(t *testing.T) {
	got := normalizeGroupIDs([]string{" beta ", "alpha", "", "alpha", " beta"})
	want := []string{"alpha", "beta"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeGroupIDs() = %#v, want %#v", got, want)
	}
}
