package controller

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/oxidns"
)

func TestNeedsNativeDNSRestart(t *testing.T) {
	tests := []struct {
		name   string
		before oxidns.Settings
		after  oxidns.Settings
		want   bool
	}{
		{name: "enable managed DNS", after: oxidns.Settings{Enabled: true, UseSingBox: true}, want: true},
		{name: "change upstream only", before: oxidns.Settings{Enabled: true, UseSingBox: true, Upstream: "google"}, after: oxidns.Settings{Enabled: true, UseSingBox: true, Upstream: "yandex-safe"}, want: false},
		{name: "change DoH port only", before: oxidns.Settings{Enabled: true, UseSingBox: true, Port: 8443}, after: oxidns.Settings{Enabled: true, UseSingBox: true, Port: 443}, want: false},
		{name: "disable managed DNS", before: oxidns.Settings{Enabled: true, UseSingBox: true}, want: true},
		{name: "enable standalone DoH only", after: oxidns.Settings{Enabled: true}, want: false},
		{name: "disable standalone DoH only", before: oxidns.Settings{Enabled: true}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if actual := needsNativeDNSRestart(tc.before, tc.after); actual != tc.want {
				t.Fatalf("needsNativeDNSRestart() = %v, want %v", actual, tc.want)
			}
		})
	}
}
