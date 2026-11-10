package controller

import (
	"context"
	"testing"
	"time"
)

func TestCancelledPanelBackgroundWorkersAlwaysJoin(t *testing.T) {
	// An aborted start must not access the database or leave workers behind.
	// New controller instances must retain that guarantee after panel restarts.
	for range 3 {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		controller := &APIController{
			firewallController: &FirewallController{},
			adBlockController:  &AdBlockController{},
		}
		controller.StartBackground(ctx)
		done := make(chan struct{})
		go func() { controller.WaitBackground(); close(done) }()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("cancelled panel workers did not stop")
		}
	}
}
