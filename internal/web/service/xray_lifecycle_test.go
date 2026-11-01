package service

import (
	"sync"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

func TestXrayLifecycleSnapshotDoesNotOverwriteNewerResult(t *testing.T) {
	state := presenceState{}
	first := xray.NewPresence()
	second := xray.NewPresence()

	state.replace(first)
	state.storeResult(first, "first result")
	process, result := state.snapshot()
	if process != first || result != "first result" {
		t.Fatalf("snapshot = (%p, %q), want (%p, %q)", process, result, first, "first result")
	}
	state.replace(second)
	state.storeResult(first, "old result")

	process, result = state.snapshot()
	if process != second {
		t.Fatal("snapshot returned the replaced process")
	}
	if result != "" {
		t.Fatalf("snapshot result = %q, want empty", result)
	}
}

func TestXrayLifecycleConcurrentStatusResultAndTrafficReads(t *testing.T) {
	previousProcess, previousResult := presenceRegistry.snapshot()
	t.Cleanup(func() {
		presenceRegistry.mu.Lock()
		presenceRegistry.process = previousProcess
		presenceRegistry.result = previousResult
		presenceRegistry.mu.Unlock()
	})

	first := xray.NewPresence()
	second := xray.NewPresence()
	service := CoreService{}
	var wg sync.WaitGroup

	wg.Go(func() {
		for range 200 {
			presenceRegistry.replace(first)
			presenceRegistry.replace(second)
		}
	})

	for range 4 {
		wg.Go(func() {
			for range 200 {
				_ = service.IsCoreRunning()
				_ = service.GetCoreResult()
				_ = service.GetCoreState()
			}
		})
	}

	wg.Wait()
}
