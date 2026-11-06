package service

import (
	"testing"
	"testing/synctest"
	"time"
)

func TestStaleStatusDoesNotWaitForBlockedCollector(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		previous := &Status{T: time.Now().Add(-time.Minute)}
		svc := &ServerService{lastStatus: previous}
		// Simulate a sample blocked in a core lock or system metrics call.
		svc.refreshStatusMu.Lock()
		defer svc.refreshStatusMu.Unlock()
		start := time.Now()
		for range 20 {
			if got := svc.CurrentStatus(); got != previous {
				t.Fatal("status request lost the last successful snapshot")
			}
		}
		synctest.Wait()
		if time.Since(start) != 0 {
			t.Fatal("cached requests waited for the blocked collector")
		}
		if svc.statusRefreshPending.Load() {
			t.Fatal("refresh request remained queued behind the blocked collector")
		}
		if got := svc.RefreshStatus(); got != previous {
			t.Fatal("overlapping cron sample should return the existing snapshot")
		}
	})
}
