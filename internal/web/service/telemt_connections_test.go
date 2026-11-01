package service

import (
	"encoding/json"
	"testing"
)

func TestNormalizeTelemtIP(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "ipv4", in: "203.0.113.10", want: "203.0.113.10"},
		{name: "ipv4 with port", in: "203.0.113.10:44321", want: "203.0.113.10"},
		{name: "ipv6 with port", in: "[2001:db8::10]:44321", want: "2001:db8::10"},
		{name: "invalid", in: "not-an-ip", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeTelemtIP(tt.in); got != tt.want {
				t.Fatalf("normalizeTelemtIP(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeTelemtIPs(t *testing.T) {
	got := normalizeTelemtIPs([]string{
		"203.0.113.11",
		"203.0.113.10",
		"203.0.113.11",
		"not-an-ip",
	})

	want := []string{"203.0.113.10", "203.0.113.11"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}

func TestCollectTelemtSnapshotIPs(t *testing.T) {
	users := []telemtUserStats{
		{
			Username:           "alice",
			CurrentConnections: 2,
			ActiveUniqueIPs:    []string{"203.0.113.10", "203.0.113.11"},
		},
		{
			Username:           "bob",
			CurrentConnections: 1,
			ActiveUniqueIPs:    []string{"203.0.113.11", "203.0.113.12"},
			RecentUniqueIPs:    []string{"198.51.100.7"},
		},
	}

	got := collectTelemtSnapshotIPs(users)
	if len(got) != 3 {
		t.Fatalf("got %#v, want three unique active IPs", got)
	}
	for _, want := range []string{"203.0.113.10", "203.0.113.11", "203.0.113.12"} {
		found := false
		for _, ip := range got {
			if ip == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("IP %s is missing from %#v", want, got)
		}
	}
}

func TestDecodeTelemtIPs(t *testing.T) {
	got := decodeTelemtIPs("[\"203.0.113.11\",\"203.0.113.10\",\"203.0.113.11\"]")
	want := []string{"203.0.113.10", "203.0.113.11"}

	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %#v, want %#v", got, want)
		}
	}
}

func TestMaxInt64(t *testing.T) {
	if got := maxInt64(-1, 0); got != 0 {
		t.Fatalf("maxInt64(-1, 0) = %d, want 0", got)
	}
	if got := maxInt64(42, 0); got != 42 {
		t.Fatalf("maxInt64(42, 0) = %d, want 42", got)
	}
}

func TestTelemtConnectionActiveIPsJSONIsArray(t *testing.T) {
	payload, err := json.Marshal(TelemtConnection{
		Username:  "inactive",
		ActiveIPs: make([]TelemtIPLocation, 0),
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got := string(decoded["activeIPs"]); got != "[]" {
		t.Fatalf("activeIPs JSON = %s, want []", got)
	}
}
