package service

import (
	"context"
	"os"
)

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

// RecoverIfCrashed rechecks selection and manual Stop under the lifecycle lock.
// A watchdog observation is stale as soon as an operator changes either one.
func (s *SingBoxService) RecoverIfCrashed(ctx context.Context) error {
	if CoreSwitchInProgress() {
		return nil
	}
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	coreType, err := (&SettingService{}).GetCoreType()
	if err != nil {
		return err
	}
	if CoreSwitchInProgress() || coreType != s.coreType() || !s.DidCrash() {
		return nil
	}
	if s.coreType() == CoreTypeXray {
		return s.restartStandaloneXrayLocked(ctx)
	}
	return s.restartLocked(ctx)
}
