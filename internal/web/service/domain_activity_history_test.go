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
		if !ValidDomainActivityRetentionSeconds(v) {
			t.Fatalf("valid interval %d rejected", v)
		}
	}
	for _, v := range []int{-1, 0, 9, 86401} {
		if ValidDomainActivityRetentionSeconds(v) {
			t.Fatalf("invalid interval %d accepted", v)
		}
	}
}

func TestDomainActivityLateSnapshotCannotRestoreHistory(t *testing.T) {
	ClearDomainActivityHistory()
	t.Cleanup(ClearDomainActivityHistory)
	now := time.Now()
	generation := DomainActivityHistoryGeneration()

	// The API call started in the old epoch. Turning monitoring off must
	// make its eventual result ineligible for storage.
	ClearDomainActivityHistory()
	entry := DomainActivityEntry{
		Email: "client@example.test", Domain: "example.org", LastSeen: now, Count: 1,
	}
	if RecordDomainActivityIfGeneration(CoreTypeSingBox, []DomainActivityEntry{entry}, now, 300, generation) {
		t.Fatal("stale snapshot was accepted after monitoring disabled")
	}
	if items := DomainActivityHistoryEntries(CoreTypeSingBox, now, 300); len(items) != 0 {
		t.Fatalf("disabled history was restored: %+v", items)
	}

	// A newly enabled monitoring epoch may record fresh observations.
	newGeneration := DomainActivityHistoryGeneration()
	if !RecordDomainActivityIfGeneration(CoreTypeSingBox, []DomainActivityEntry{entry}, now, 300, newGeneration) {
		t.Fatal("current snapshot was rejected")
	}
}
