package service

import (
	"os"
	"sync/atomic"
)

var isSingBoxManuallyStopped atomic.Bool

// Installed reports whether the configured sing-box binary exists as a
// regular filesystem entry. It deliberately avoids executing the binary,
// so status/watchdog checks stay cheap when sing-box is not installed.
func (s *SingBoxService) Installed() bool {
	info, err := os.Stat(s.BinaryPath())
	return err == nil && !info.IsDir()
}

// DidCrash distinguishes an unexpected process exit from the two normal
// stopped states: the operator explicitly stopped sing-box, or sing-box
// is not installed on this panel/node at all.
func (s *SingBoxService) DidCrash() bool {
	return s.Installed() && !s.IsRunning() && !s.stoppedFlag().Load()
}

func markSingBoxStarted() {
	isSingBoxManuallyStopped.Store(false)
}

func markSingBoxStopped() {
	isSingBoxManuallyStopped.Store(true)
}
