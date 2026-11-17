package controller

import (
	"context"
	"testing"
	"testing/synctest"
	"time"
)

func TestDevUpdateChecksStopWithPanel(t *testing.T) {
	for _, beforeFirst := range []bool{true, false} {
		t.Run(map[bool]string{true: "before first check", false: "after first check"}[beforeFirst], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				calls := 0
				done := make(chan struct{})
				go func() { defer close(done); runDevUpdateChecks(ctx, func() error { calls++; return nil }) }()
				synctest.Wait()
				if !beforeFirst {
					time.Sleep(15 * time.Second)
					synctest.Wait()
					if calls != 1 {
						t.Fatalf("initial checks=%d", calls)
					}
				}
				cancel()
				synctest.Wait()
				select {
				case <-done:
				default:
					t.Fatal("retired panel updater is still running")
				}
				time.Sleep(10 * time.Minute)
				synctest.Wait()
				want := 0
				if !beforeFirst {
					want = 1
				}
				if calls != want {
					t.Fatalf("retired panel ran updates: calls=%d want=%d", calls, want)
				}
			})
		})
	}
}
