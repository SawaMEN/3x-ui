package service

import (
	"testing"
	"time"
)

func TestDomainFromAccessDestination(t *testing.T) {
	tests := map[string]string{
		"tcp:Example.COM:443":     "example.com",
		"udp:api.example.com:443": "api.example.com",
		"sub.example.org.:80":     "sub.example.org",
		"8.8.8.8:443":             "",
		"[2001:db8::1]:443":       "",
		"localhost:80":            "",
		"example.com:0":           "",
		"example.com:99999":       "",
		"example.com:abc":         "",
		"example.com:443/secret":  "",
		"bad_site.example:443":    "",
		"-bad.example:443":        "",
		"example.com":             "",
	}
	for input, want := range tests {
		if got := domainFromAccessDestination(input); got != want {
			t.Errorf("domainFromAccessDestination(%q) = %q; want %q", input, got, want)
		}
	}
}

func TestAggregateRecentDomainActivity(t *testing.T) {
	now := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	logs := []LogEntry{
		{Email: "alice", ToAddress: "tcp:YouTube.com:443", DateTime: now.Add(-10 * time.Second)},
		{Email: "alice", ToAddress: "youtube.com:443", DateTime: now.Add(-5 * time.Second)},
		{Email: "bob", ToAddress: "example.com:443", DateTime: now.Add(-time.Minute)},
		{Email: "alice", ToAddress: "private.example:443", DateTime: now.Add(-3 * time.Minute)},
		{Email: "alice", ToAddress: "blocked.example:443", DateTime: now, Event: 1},
		{Email: "alice", ToAddress: "8.8.8.8:443", DateTime: now},
		{Email: "", ToAddress: "anonymous.example:443", DateTime: now},
		{Email: "future", ToAddress: "future.example:443", DateTime: now.Add(time.Minute)},
	}
	got := aggregateRecentDomainActivity(logs, now)
	if len(got) != 2 {
		t.Fatalf("got %d entries, want 2: %+v", len(got), got)
	}
	if got[0].Email != "alice" || got[0].Domain != "youtube.com" || got[0].Count != 2 ||
		!got[0].LastSeen.Equal(now.Add(-5*time.Second)) {
		t.Fatalf("unexpected newest activity: %+v", got[0])
	}
	if got[1].Email != "bob" || got[1].Domain != "example.com" {
		t.Fatalf("unexpected second activity: %+v", got[1])
	}
}
