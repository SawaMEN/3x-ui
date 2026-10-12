package service

import "github.com/SawaMEN/3x-ui/v3/internal/xray"

// SetOnlinePresenceForTest installs a presence tracker and returns the restore func,
// so tests in other packages can observe online state. Never call it in production.
func SetOnlinePresenceForTest(p *xray.Presence) (restore func()) {
	previousProcess, previousResult := presenceRegistry.snapshot()
	presenceRegistry.replace(p)
	return func() {
		presenceRegistry.mu.Lock()
		presenceRegistry.process = previousProcess
		presenceRegistry.result = previousResult
		presenceRegistry.mu.Unlock()
	}
}
