package version

import "testing"

func TestCompareRejectsUnexpectedFormats(t *testing.T) {
	for _, invalid := range []string{"-1.2.3", "1.-2.3", "1.2.-3", "+1.2.3", "1.+2.3", "1.2.+3", "1. 2.3", "1.2.３"} {
		if _, ok := Compare(invalid, "2.9.3"); ok {
			t.Errorf("accepted invalid version %q", invalid)
		}
	}
	if _, ok := Compare("latest", "2.9.3"); ok {
		t.Fatal("expected non-semver latest tag to be rejected")
	}
	if _, ok := Compare("v2.9", "2.9.3"); ok {
		t.Fatal("expected short version to be rejected")
	}
}
