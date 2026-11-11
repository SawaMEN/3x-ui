package websocket

import (
	"sync"
	"testing"
	"time"
)

func assertAuditClientClosed(t *testing.T, c *Client) {
	t.Helper()
	select {
	case _, ok := <-c.Send:
		if ok {
			t.Fatal("unexpected buffered message")
		}
	default:
		t.Fatal("client send channel remains open after hub shutdown")
	}
}

func TestAuditHubShutdownClosesPendingRegistrations(t *testing.T) {
	h := NewHub()
	client := NewClient("pending")
	h.Register(client) // Queued, but not yet in h.clients.
	h.Stop()
	h.shutdown()
	assertAuditClientClosed(t, client)
	if len(h.ops) != 0 {
		t.Fatal("shutdown retained queued connection operations")
	}
}

func TestAuditHubRegisterAfterStopClosesConnection(t *testing.T) {
	h := NewHub()
	h.Stop()
	h.shutdown()
	client := NewClient("too-late")
	h.Register(client)
	assertAuditClientClosed(t, client)
}

func TestAuditHubConcurrentRegisterAndStop(t *testing.T) {
	for round := 0; round < 30; round++ {
		h := NewHub()
		stopped := make(chan struct{})
		go func() { h.Run(); close(stopped) }()
		clients := make([]*Client, 64)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := range clients {
			clients[i] = NewClient("shutdown-race")
			wg.Add(1)
			go func(c *Client) {
				defer wg.Done()
				<-start
				h.Register(c)
			}(clients[i])
		}
		close(start)
		h.Stop()
		wg.Wait()
		select {
		case <-stopped:
		case <-time.After(3 * time.Second):
			t.Fatal("hub did not stop")
		}
		for _, c := range clients {
			assertAuditClientClosed(t, c)
		}
	}
}
