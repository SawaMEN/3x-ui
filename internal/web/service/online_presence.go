package service

import "github.com/SawaMEN/3x-ui/v3/internal/xray"

var onlinePresenceFallback = xray.NewPresence()

// EnsureOnlinePresenceTracker keeps local and remote presence independently of runtime state.
func EnsureOnlinePresenceTracker() {
	if currentOnlinePresence() != nil {
		return
	}
	presenceRegistry.mu.Lock()
	if presenceRegistry.process == nil {
		presenceRegistry.process = onlinePresenceFallback
	}
	presenceRegistry.mu.Unlock()
}
