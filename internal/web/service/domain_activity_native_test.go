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
	if len(got) != 2 {
		t.Fatalf("entries = %d, want 2: %+v", len(got), got)
	}
	if got[0].Email != "alice" || got[0].Domain != "youtube.com" ||
		got[0].Count != 2 || !got[0].LastSeen.Equal(now) {
		t.Errorf("unexpected first row: %+v", got[0])
	}
	if got[1].Email != "bob" || got[1].Domain != "github.com" || got[1].Count != 1 {
		t.Errorf("unexpected second row: %+v", got[1])
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
