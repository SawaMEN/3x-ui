package job

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestMtprotoWebJobOwnsWorkAndCancel(t *testing.T) {
	setupIntegrationDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	j := NewMtprotoWebJob(ctx)
	entered, seenCancel, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	j.reconcile = func(ctx context.Context) error {
		calls.Add(1)
		close(entered)
		<-ctx.Done()
		close(seenCancel)
		<-release
		return ctx.Err()
	}
	done := make(chan struct{})
	go func() { j.Run(); close(done) }()
	<-entered
	// An overlapping tick must not create a second certificate/config worker.
	j.Run()
	cancel()
	<-seenCancel
	select {
	case <-done:
		t.Fatal("job returned before reconciliation released resources")
	default:
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("job did not join reconciliation")
	}
	j.Run()
	if calls.Load() != 1 {
		t.Fatalf("overlap/canceled tick started %d workers", calls.Load())
	}
}
