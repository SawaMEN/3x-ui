package job

import (
	"context"
	"testing"
	"time"
)

func TestCPUSampleCancelledOnPanelRestart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		NewCheckCpuJobWithContext(ctx).Run()
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("CPU sampler delays panel restart after cancellation")
	}
}
