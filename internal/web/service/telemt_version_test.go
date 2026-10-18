package service

import "testing"

func TestNormalizeTelemtVersion(t *testing.T) {
	tests := map[string]string{
		"0.7.13":                 "0.7.13",
		"v0.7.13":                "0.7.13",
		"Telemt 0.7.13":          "0.7.13",
		"telemt version v0.7.13": "0.7.13",
		"telemt 0.7.13+build.1":  "0.7.13+build.1",
		"  v0.7.13  ":            "0.7.13",
	}
	for input, want := range tests {
		if got := normalizeTelemtVersion(input); got != want {
			t.Fatalf("normalizeTelemtVersion(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestTelemtVersionEqualityDoesNotReportUpdate(t *testing.T) {
	current := "Telemt 0.7.13"
	latest := "v0.7.13"
	if normalizeTelemtVersion(current) != normalizeTelemtVersion(latest) {
		t.Fatalf("equal Telemt versions were normalized differently: %q vs %q", current, latest)
	}
}

func TestTelemtNeedsUpdate(t *testing.T) {
	cases := []struct {
		name, current, latest string
		want bool
	}{
		{"new upstream", "telemt 3.5.13", "3.5.14", true},
		{"major version", "telemt 2.9.99", "3.0.0", true},
		{"up to date", "telemt 3.5.14", "3.5.14", false},
		{"v prefix", "telemt v3.5.14", "v3.5.14", false},
		{"legacy patched migration", "telemt 3.5.14+3xui.1", "3.5.14", true},
		{"patched build newer than upstream", "telemt 3.5.15+3xui.1", "3.5.14", false},
		{"never downgrade", "telemt 3.6.0", "3.5.14", false},
		{"missing current", "", "3.5.14", false},
		{"missing latest", "3.5.14", "", false},
		{"invalid", "not a version", "3.5.14", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := telemtNeedsUpdate(tc.current, tc.latest); got != tc.want {
				t.Fatalf("telemtNeedsUpdate(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
			}
		})
	}
}
