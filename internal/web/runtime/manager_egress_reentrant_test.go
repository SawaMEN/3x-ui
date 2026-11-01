package runtime

import (
	"testing"
	"time"
)

type reentrantNodeEgressResolver struct {
	manager *Manager
}

func (r reentrantNodeEgressResolver) NodeEgressProxyURL(nodeID int) string {
	// Changing the resolver is a legitimate callback into Manager. Holding
	// Manager.mu while calling this method would deadlock on its write lock.
	r.manager.SetNodeEgressResolver(nil)
	if nodeID != 7 {
		return ""
	}
	return "http://127.0.0.1:1080"
}

func TestManagerEgressResolverMayReenterManager(t *testing.T) {
	manager := NewManager(LocalDeps{})
	manager.SetNodeEgressResolver(reentrantNodeEgressResolver{manager: manager})
	done := make(chan string, 1)
	go func() {
		done <- manager.NodeEgressProxyURL(7)
	}()
	select {
	case got := <-done:
		if got != "http://127.0.0.1:1080" {
			t.Fatalf("egress resolver returned %q", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("egress resolver callback deadlocked on Manager.mu")
	}
	if got := manager.NodeEgressProxyURL(7); got != "" {
		t.Fatalf("stale resolver used after clearing: %q", got)
	}
}
