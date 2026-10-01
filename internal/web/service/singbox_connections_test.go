package service

import "testing"

func TestSingBoxConnectionMatches(t *testing.T) {
	item := SingBoxConnectionInfo{
		User:     "alice",
		Inbound:  "in-vless",
		Outbound: "proxy-a",
		Chain:    []string{"selector", "proxy-a"},
	}
	for _, tc := range []struct {
		name     string
		resource string
		tag      string
		want     bool
	}{
		{name: "all", want: true},
		{name: "user", resource: "user", tag: "alice", want: true},
		{name: "inbound", resource: "inbound", tag: "in-vless", want: true},
		{name: "outbound", resource: "outbound", tag: "proxy-a", want: true},
		{name: "outbound chain", resource: "outbound", tag: "selector", want: true},
		{name: "miss", resource: "user", tag: "bob", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := singBoxConnectionMatches(item, tc.resource, tc.tag); got != tc.want {
				t.Fatalf("match = %v, want %v", got, tc.want)
			}
		})
	}
}
