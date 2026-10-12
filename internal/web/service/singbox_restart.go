package service

import (
	"bytes"
	"context"
	"errors"
	"os"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

var nativePendingApplyMu sync.Mutex

// SetToNeedRestart marks the selected sing-box runtime config as stale.
// Inbound/client controllers use the same deferred-restart model as Xray
// so a burst of node-sync mutations is collapsed into one core reload.
func (s *SingBoxService) SetToNeedRestart() {
	s.restartFlag().Store(true)
}

// IsNeedRestartAndSetFalse consumes the pending sing-box restart flag.
func (s *SingBoxService) IsNeedRestartAndSetFalse() bool {
	return s.restartFlag().CompareAndSwap(true, false)
}

// ApplyPendingRestart reloads sing-box only while it is the selected,
// currently running core. If sing-box is absent/stopped there is no
// process to reload; a later explicit start writes the current config.
func (s *SingBoxService) ApplyPendingRestart(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	// Another cron tick must not consume a flag raised during an in-flight apply.
	if !nativePendingApplyMu.TryLock() {
		return
	}
	defer nativePendingApplyMu.Unlock()
	if batch := localConfigBatch(); batch != nil && batch.ConfigBatchActive() {
		return
	}
	if !s.IsNeedRestartAndSetFalse() {
		return
	}
	coreType, err := (&SettingService{}).GetCoreType()
	if err != nil {
		logger.Error("get selected core for pending sing-box restart failed:", err)
		s.SetToNeedRestart()
		return
	}
	if coreType != s.coreType() || !s.IsRunning() {
		return
	}
	// Apply the final DB state under the same lock as Start/Stop/manual restart.
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	// Stop may have won the lock after the initial running check. Never undo a
	// manual stop, and never publish an intermediate active batch after waiting.
	if ctx.Err() != nil {
		s.SetToNeedRestart()
		return
	}
	coreType, err = (&SettingService{}).GetCoreType()
	if err != nil {
		s.SetToNeedRestart()
		return
	}
	if coreType != s.coreType() || !s.IsRunning() {
		return
	}
	if batch := localConfigBatch(); batch != nil && batch.ConfigBatchActive() {
		s.SetToNeedRestart()
		return
	}
	{
		cfg, err := s.GetConfig()
		if err != nil {
			s.process().SetError(err)
			s.SetToNeedRestart()
			return
		}
		data, err := cfg.Marshal()
		if err != nil {
			s.process().SetError(err)
			s.SetToNeedRestart()
			return
		}
		if bytes.Equal(data, s.process().AppliedConfig()) && externalvpn.OutboundsApplied(cfg.ManagedOutbounds) {
			// WriteConfig/editor may have published another candidate without applying
			// it. Restore the actual applied bytes rather than treating disk as runtime.
			if disk, err := os.ReadFile(s.ProcessConfigPath()); err != nil || !bytes.Equal(disk, data) {
				if err := writeNativeConfigBytes(s.ProcessConfigPath(), data); err != nil {
					s.process().SetError(err)
					s.SetToNeedRestart()
					return
				}
			}
			s.process().SetError(nil)
			return
		}
	}
	if err := s.restartLocked(ctx); err != nil {
		s.SetToNeedRestart()
		if errors.Is(err, errNativeConfigBatchChanged) {
			return
		}
		s.process().SetError(err)
		logger.Error("apply native core configuration failed:", err)
	}
}
