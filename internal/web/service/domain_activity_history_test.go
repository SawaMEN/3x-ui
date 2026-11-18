package service

import (
	"testing"
	"time"
)

func TestDomainActivityHistoryRetention(t *testing.T) {
	ClearDomainActivityHistory()
	t.Cleanup(ClearDomainActivityHistory)
	now := time.Now().Truncate(time.Second)
	seen := DomainActivityEntry{
		Email: "client@example.test", Domain: "example.org", LastSeen: now, Count: 2,
	}
	RecordDomainActivity(CoreTypeSingBox, []DomainActivityEntry{seen}, now, 300)
	items := DomainActivityHistoryEntries(CoreTypeSingBox, now.Add(2*time.Minute), 300)
	if len(items) != 1 || items[0].Count != 2 {
		t.Fatalf("history after two minutes = %#v", items)
	}
	// A repeated API snapshot must not double-count a connection.
	RecordDomainActivity(CoreTypeSingBox, []DomainActivityEntry{seen}, now.Add(2*time.Minute), 300)
	items = DomainActivityHistoryEntries(CoreTypeSingBox, now.Add(2*time.Minute), 300)
	if len(items) != 1 || items[0].Count != 2 {
		t.Fatalf("repeated snapshot duplicated activity = %#v", items)
	}
	if got := DomainActivityHistoryEntries(CoreTypeSingBox, now.Add(6*time.Minute), 300); len(got) != 0 {
		t.Fatalf("expired domain still visible = %#v", got)
	}
}

func TestDomainActivityHistoryCoreIsolationAndDisabledClear(t *testing.T) {
	ClearDomainActivityHistory()
	t.Cleanup(ClearDomainActivityHistory)
	now := time.Now()
	RecordDomainActivity(CoreTypeSingBox, []DomainActivityEntry{
		{Email: "alice", Domain: "example.org", LastSeen: now, Count: 1},
	}, now, 300)
	if got := DomainActivityHistoryEntries(CoreTypeXray, now, 300); len(got) != 0 {
		t.Fatalf("other core inherited history = %#v", got)
	}
	RecordDomainActivity(CoreTypeXray, []DomainActivityEntry{
		{Email: "bob", Domain: "example.net", LastSeen: now, Count: 1},
	}, now, 300)
	ClearDomainActivityHistory()
	if got := DomainActivityHistoryEntries(CoreTypeXray, now, 300); len(got) != 0 {
		t.Fatalf("history survived disable = %#v", got)
	}
}

func TestDomainActivityHistoryWindowBounds(t *testing.T) {
	for _, v := range []int{10, 30, 60, 300, 86400} {
		if !ValidDomainActivityRetentionSeconds(v) { t.Fatalf("valid interval %d rejected", v) }
	}
	for _, v := range []int{-1, 0, 9, 86401} {
		if ValidDomainActivityRetentionSeconds(v) { t.Fatalf("invalid interval %d accepted", v) }
	}
}
