package web

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/op/go-logging"
	"github.com/robfig/cron/v3"

	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

func TestPanelStopJoinsInitialScheduledJob(t *testing.T) {
	t.Setenv("XUI_DB_FOLDER", t.TempDir())
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	s := NewServer(nil)
	s.cron = cron.New(cron.WithChain(cron.Recover(cron.DiscardLogger)))
	entered, canceled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	exited := make(chan struct{})
	scheduleStartupJob(s.cron, "@every 1h", cron.FuncJob(func() {
		defer close(exited)
		close(entered)
		<-s.ctx.Done()
		close(canceled)
		<-release
	}), s.runStartupTask)
	<-entered
	stopped := make(chan error, 1)
	go func() { stopped <- s.StopPanelOnly() }()
	<-canceled
	select {
	case err := <-stopped:
		close(release)
		<-exited
		t.Fatalf("panel returned before initial job released DB/resources: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-stopped; err != nil {
		t.Fatal(err)
	}
}

func TestPanelTaskTimeoutStillRetainsJoin(t *testing.T) {
	logger.InitLogger(logging.ERROR)
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitPanelTask(ctx, "synthetic startup", func() { <-done }); err == nil {
		t.Fatal("expected canceled panel reload wait")
	}
	waited := make(chan struct{})
	go func() { retiredPanelTasks.Wait(); close(waited) }()
	select {
	case <-waited:
		t.Fatal("retired worker was not retained")
	default:
	}
	close(done)
	select {
	case <-waited:
	case <-time.After(time.Second):
		t.Fatal("retired worker did not finish")
	}
}

func TestDelayedStartupTaskCancelsWithoutRunning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := NewServer(nil)
		var calls atomic.Int32
		s.runDelayedStartupTask(time.Hour, func() { calls.Add(1) })
		// Let the worker start and block inside its timer before cancellation.
		synctest.Wait()
		s.cancel()
		s.startupTasks.Wait()
		if calls.Load() != 0 {
			t.Fatal("stopped panel ran delayed memory release")
		}
	})
}

func TestTrackedStartupPanicReleasesJoin(t *testing.T) {
	s := NewServer(nil)
	s.cron = cron.New(cron.WithChain(cron.Recover(cron.DiscardLogger)))
	scheduleStartupJob(s.cron, "@every 1h", cron.FuncJob(func() { panic("synthetic startup failure") }), s.runStartupTask)
	done := make(chan struct{})
	go func() { s.startupTasks.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("panic retained initial task in shutdown join")
	}
	s.cancel()
}
