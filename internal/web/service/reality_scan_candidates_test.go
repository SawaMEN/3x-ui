package service

import (
	"strings"
	"testing"
)

func TestRealityScanExpandedDefaultsPreserveCustomLists(t *testing.T) {
	setupBulkDB(t)
	svc := &SettingService{}
	cases := []struct{ name, stored, want string }{
		{"legacy default", strings.Join(legacyRealityScanCandidates, ","), DefaultRealityScanCandidatesCSV},
		{"previous default", strings.Join(previousRealityScanCandidates, ","), DefaultRealityScanCandidatesCSV},
		{"modified previous default", strings.Join(previousRealityScanCandidates[:59], ","), strings.Join(previousRealityScanCandidates[:59], ",")},
		{"custom list", "custom.example:8443,203.0.113.0/30", "custom.example:8443,203.0.113.0/30"},
		{"empty fallback", "", ""},
	}
	if len(defaultRealityScanCandidates) != 160 {
		t.Fatalf("expected 160 seeds, got %d", len(defaultRealityScanCandidates))
	}
	seen := map[string]bool{}
	for _, target := range defaultRealityScanCandidates {
		if seen[target] {
			t.Fatalf("duplicate %s", target)
		}
		seen[target] = true
		if _, port, err := splitRealityTarget(target); err != nil || port != 443 {
			t.Fatalf("invalid seed %s: %v", target, err)
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := svc.setString("realityScanCandidates", tc.stored); err != nil {
				t.Fatal(err)
			}
			got, err := svc.GetRealityScanCandidates()
			if err != nil || got != tc.want {
				t.Fatalf("getter: %q %v", got, err)
			}
			all, err := svc.GetAllSetting()
			if err != nil || all.RealityScanCandidates != tc.want {
				t.Fatalf("settings UI: %v %+v", err, all)
			}
			tokens := (&ServerService{}).realityScanCandidateTokens()
			if tc.stored == "" && len(tokens) != 160 {
				t.Fatal("empty list did not use expanded seeds")
			}
		})
	}
}
