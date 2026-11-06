package websocket

import (
	"sync"
	"testing"
	"time"
)

// Run fanout concurrently with disconnects and shutdown under -race.
// The hub must not send on a channel after Unregister closes it.
func TestStage12ConcurrentFanoutAndUnregister(t *testing.T) {
	h := NewHub()
	stopped := make(chan struct{})
	go func() {
		h.Run()
		close(stopped)
	}()
	defer func() {
		h.Stop()
		<-stopped
	}()

	const clients = 96
	all := make([]*Client, clients)
	for i := range all {
		c := NewClient("stress")
		all[i] = c
		h.Register(c)
	}
	waitClientCount(t, h, clients)

	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			for j := 0; j < 80; j++ {
				h.fanout([]byte("live update"))
			}
		}()
	}
	for _, c := range all {
		wg.Add(1)
		go func(client *Client) {
			defer wg.Done()
			<-start
			h.Unregister(client)
		}(c)
	}
	close(start)

	finished := make(chan struct{})
	go func() {
		wg.Wait()
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(10 * time.Second):
		t.Fatal("fanout and unregister workers deadlocked")
	}
	waitClientCount(t, h, 0)
	for _, c := range all {
		// A send buffer can contain earlier snapshots, but the channel must
		// eventually be closed, even if the client was evicted for backpressure.
		closed := false
		for !closed {
			select {
			case _, ok := <-c.Send:
				closed = !ok
			default:
				t.Fatal("client channel remained open after unregister")
			}
		}
	}
}

func TestStage12HubCancelPendingThrottleTimers(t *testing.T) {
	for round := 0; round < 100; round++ {
		h := NewHub()
		if h.shouldThrottle(MessageTypeTraffic) {
			t.Fatalf("round %d: first traffic message should not throttle", round)
		}
		if !h.shouldThrottle(MessageTypeTraffic) {
			t.Fatalf("round %d: second traffic message should schedule invalidation", round)
		}
		h.Stop()
		h.throttleMu.Lock()
		pending := len(h.pendingInvalidations)
		h.throttleMu.Unlock()
		if pending != 0 {
			t.Fatalf("round %d: %d invalidation timers retained after Stop", round, pending)
		}
	}
}
