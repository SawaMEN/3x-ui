package service

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// A restart request can race with StopTrafficWriter while its previous
// generation is still draining writes. Start must wait for that generation
// instead of returning successfully and leaving the writer permanently off.
func TestStage12TrafficWriterRestartDuringStopDrain(t *testing.T) {
	resetTrafficWriterForTest(t)
	StartTrafficWriter()

	writeStarted := make(chan struct{})
	releaseWrite := make(chan struct{})
	defer func() {
		select {
		case <-releaseWrite:
		default:
			close(releaseWrite)
		}
	}()

	writeErr := make(chan error, 1)
	go func() {
		writeErr <- submitTrafficWrite(func() error {
			close(writeStarted)
			<-releaseWrite
			return nil
		})
	}()
	waitTrafficWriterSignal(t, writeStarted, "blocked write never started")

	stopDone := make(chan struct{})
	go func() {
		StopTrafficWriter()
		close(stopDone)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		twMu.Lock()
		stopping := twCtx != nil && twCtx.Err() != nil
		twMu.Unlock()
		if stopping {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("StopTrafficWriter never cancelled the previous generation")
		}
		time.Sleep(time.Millisecond)
	}

	restartDone := make(chan struct{})
	go func() {
		StartTrafficWriter()
		close(restartDone)
	}()
	select {
	case <-restartDone:
		t.Fatal("StartTrafficWriter returned before the cancelled writer drained its in-flight write")
	case <-time.After(30 * time.Millisecond):
	}

	close(releaseWrite)
	waitTrafficWriterSignal(t, stopDone, "StopTrafficWriter did not drain")
	waitTrafficWriterSignal(t, restartDone, "concurrent StartTrafficWriter did not restart")
	if err := waitTrafficWriterErr(t, writeErr); err != nil {
		t.Fatalf("original write: %v", err)
	}

	twMu.Lock()
	active := twCtx != nil && twCtx.Err() == nil && twDone != nil
	twMu.Unlock()
	if !active {
		t.Fatal("restart request was lost: traffic writer remains stopped")
	}
	if err := submitTrafficWrite(func() error { return nil }); err != nil {
		t.Fatalf("restarted writer failed to accept work: %v", err)
	}
}

// Repeat generation changes and burst submissions under -race. This verifies
// exact execution counts; a timeout cannot be mistaken for a successful soak.
func TestStage12TrafficWriterBurstAcrossRestarts(t *testing.T) {
	resetTrafficWriterForTest(t)

	const cycles = 64
	const workers = 16
	var completed atomic.Int64

	for cycle := range cycles {
		StartTrafficWriter()
		var wg sync.WaitGroup
		errs := make(chan error, workers)
		for worker := range workers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				errs <- submitTrafficWrite(func() error {
					completed.Add(1)
					return nil
				})
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("cycle %d: traffic write failed: %v", cycle, err)
			}
		}
		StopTrafficWriter()
		twMu.Lock()
		cleared := twQueue == nil && twCtx == nil && twDone == nil && twCancel == nil
		twMu.Unlock()
		if !cleared {
			t.Fatalf("cycle %d: stopped writer retained a generation", cycle)
		}
	}

	if want := int64(cycles * workers); completed.Load() != want {
		t.Fatal(fmt.Sprintf("executed %d writes, expected exactly %d", completed.Load(), want))
	}
}
