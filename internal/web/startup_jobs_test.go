package web

import (
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/robfig/cron/v3"
)

func TestStartupJobSharesScheduledOverlapGuard(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger), cron.Recover(cron.DiscardLogger)))
		gate, started := make(chan struct{}), make(chan struct{})
		var calls atomic.Int32
		id := scheduleStartupJob(c, "@every 1s", cron.FuncJob(func() {
			if calls.Add(1) == 1 {
				close(started)
			}
			<-gate
		}))
		<-started
		skipped := make(chan struct{})
		go func() { c.Entry(id).WrappedJob.Run(); close(skipped) }()
		synctest.Wait()
		select {
		case <-skipped:
		default:
			t.Error("scheduled run waits on or overlaps the initial run")
		}
		if got := calls.Load(); got != 1 {
			t.Errorf("overlapping calls=%d", got)
		}
		close(gate)
		synctest.Wait()
		c.Entry(id).WrappedJob.Run()
		if got := calls.Load(); got != 2 {
			t.Fatalf("job did not resume after initial run: calls=%d", got)
		}
	})
}

func TestStartupJobPanicDoesNotCrashPanelOrDisableJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		c := cron.New(cron.WithChain(cron.SkipIfStillRunning(cron.DiscardLogger), cron.Recover(cron.DiscardLogger)))
		var calls atomic.Int32
		id := scheduleStartupJob(c, "@every 1s", cron.FuncJob(func() {
			if calls.Add(1) == 1 {
				panic("sidecar startup failed")
			}
		}))
		synctest.Wait()
		c.Entry(id).WrappedJob.Run()
		if got := calls.Load(); got != 2 {
			t.Fatalf("panic disabled subsequent job runs: calls=%d", got)
		}
	})
}
