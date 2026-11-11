package service

import (
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

func TestAggregateNativeDomainActivity(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	sessions := []singbox.ActiveSession{
		{ID: "1", User: "alice", Domain: "YouTube.COM", Destination: "1.1.1.1:443"},
		{ID: "2", User: "alice", Domain: "youtube.com", Destination: "2.2.2.2:443"},
		{ID: "3", User: "bob", Domain: "", Destination: "tcp:github.com:443"},
		{ID: "4", User: "alice", Domain: "8.8.8.8", Destination: "8.8.8.8:443"},
		{ID: "5", User: "", Domain: "unattributed.example"},
		{ID: "6", User: "alice", Domain: "bad_site.example", Destination: "4.4.4.4:443"},
		{ID: "2", User: "alice", Domain: "youtube.com"}, // duplicate snapshot ID
		{ID: "", User: "alice", Domain: "invalid.example"},
	}
	got := aggregateNativeDomainActivity(sessions, now)
	if len(got) != 3 {
		t.Fatalf("entries = %d, want 3: %+v", len(got), got)
	}
	if got[0].Email != "" || got[0].Domain != "unattributed.example" || got[0].Count != 1 {
		t.Errorf("unattributed session must not be silently dropped: %+v", got[0])
	}
	if got[1].Email != "alice" || got[1].Domain != "youtube.com" ||
		got[1].Count != 2 || !got[1].LastSeen.Equal(now) {
		t.Errorf("unexpected alice row: %+v", got[1])
	}
	if got[2].Email != "bob" || got[2].Domain != "github.com" || got[2].Count != 1 {
		t.Errorf("unexpected bob row: %+v", got[2])
	}
}

func TestAggregateNativeDomainActivityLimits(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	sessions := make([]singbox.ActiveSession, 0, 400)
	for i := range 400 {
		sessions = append(sessions, singbox.ActiveSession{
			ID: string(rune(1000 + i)), User: "client",
			Domain: "www" + string(rune('a'+i%26)) + string(rune('a'+(i/26)%26)) + ".example",
		})
	}
	got := aggregateNativeDomainActivity(sessions, now)
	if len(got) > 30 {
		t.Fatalf("too many entries for client: %d", len(got))
	}
}

func TestAggregateNativeDomainActivityHostPort(t *testing.T) {
	now := time.Unix(1700000000, 0)
	got := aggregateNativeDomainActivity([]singbox.ActiveSession{
		{ID: "one", User: "user", Domain: "EXAMPLE.org:443", Destination: "192.0.2.1:443"},
		{ID: "two", User: "user", Domain: "", Destination: "tcp:api.example.org:443"},
	}, now)
	if len(got) != 2 || got[0].Domain != "api.example.org" || got[1].Domain != "example.org" {
		t.Fatalf("unexpected Clash Host metadata normalization: %+v", got)
	}
}
