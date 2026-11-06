package job

import "testing"

func TestRejectedLDAPFetchDoesNotLowerDeletionBaseline(t *testing.T) {
	j := NewLdapSyncJob()
	if !j.acceptFetchCount(500) {
		t.Fatal("healthy first fetch rejected")
	}
	for _, count := range []int{1, 1, 0, 1, 249, 249} {
		if j.acceptFetchCount(count) {
			t.Errorf("partial fetch of %d users accepted", count)
		}
		if got := j.lastFlagCount.Load(); got != 500 {
			t.Errorf("rejected fetch lowered baseline to %d", got)
		}
	}
	if !j.acceptFetchCount(450) || j.lastFlagCount.Load() != 450 {
		t.Fatal("recovered directory did not update baseline")
	}
}

// A bind that succeeds with an unusable search returns (empty, nil); the only
// guard was `err != nil`, so that answer detached the entire inbound.
func TestAutoDeleteAllowed(t *testing.T) {
	cases := []struct {
		name     string
		previous int64
		fetched  int
		want     bool
	}{
		{"empty fetch on a fresh job is refused", 0, 0, false},
		{"empty fetch after a healthy sync is refused", 500, 0, false},
		{"full fetch on a fresh job is allowed", 0, 500, true},
		{"steady fetch is allowed", 500, 500, true},
		{"growth is allowed", 500, 900, true},
		{"shrink above the retention floor is allowed", 500, 250, true},
		{"shrink below the retention floor is refused", 500, 249, false},
		{"collapse to a single user is refused", 500, 1, false},
		{"single user with no history is allowed", 0, 1, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			j := NewLdapSyncJob()
			j.lastFlagCount.Store(c.previous)
			if got := j.autoDeleteSafeForFetch(c.fetched); got != c.want {
				t.Fatalf("autoDeleteSafeForFetch(previous=%d, fetched=%d) = %v, want %v",
					c.previous, c.fetched, got, c.want)
			}
		})
	}
}
