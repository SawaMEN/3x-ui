package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/protocolserver"
)

var nativePendingApplyMu sync.Mutex

// Protected by singBoxApplyMu; light service values share one native runtime.
var nativeApplyRetry struct {
	hash  [sha256.Size]byte
	next  time.Time
	delay time.Duration
}

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
	if CoreSwitchInProgress() {
		return
	}
	if ctx.Err() != nil {
		return
	}
	if s.coreType() == CoreTypeXray {
		singBoxApplyMu.Lock()
		defer singBoxApplyMu.Unlock()
		selected, err := (&SettingService{}).GetCoreType()
		if err != nil || selected != s.coreType() || CoreSwitchInProgress() || !s.IsRunning() {
			return
		}
		if s.IsNeedRestartAndSetFalse() {
			if err := s.restartStandaloneXrayLocked(ctx); err != nil {
				s.SetToNeedRestart()
			}
		}
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
	if !s.IsNeedRestartAndSetFalse() && !protocolserver.Changed() {
		return
	}
	coreType, err := (&SettingService{}).GetCoreType()
	if err != nil {
		logger.Error("get selected core for pending sing-box restart failed:", err)
		s.SetToNeedRestart()
		return
	}
	if CoreSwitchInProgress() || coreType != s.coreType() || !s.IsRunning() {
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
	if CoreSwitchInProgress() || coreType != s.coreType() || !s.IsRunning() {
		return
	}
	if batch := localConfigBatch(); batch != nil && batch.ConfigBatchActive() {
		s.SetToNeedRestart()
		return
	}
	var candidateHash [sha256.Size]byte
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
		if bytes.Equal(data, s.process().AppliedConfig()) && externalvpn.OutboundsApplied(cfg.ManagedOutbounds) && protocolserver.Applied(cfg.ProtocolServers) {
			// WriteConfig/editor may have published another candidate without applying
			// it. Restore the actual applied bytes rather than treating disk as runtime.
			if disk, err := os.ReadFile(s.ProcessConfigPath()); err != nil || !bytes.Equal(disk, data) {
				if err := writeNativeConfigBytes(s.ProcessConfigPath(), data); err != nil {
					s.process().SetError(err)
					s.SetToNeedRestart()
					return
				}
			}
			nativeApplyRetry.delay = 0
			s.process().SetError(nil)
			return
		}
		metadata, err := json.Marshal([]any{cfg.ManagedOutbounds, cfg.ProtocolServers})
		if err != nil {
			s.process().SetError(err)
			s.SetToNeedRestart()
			return
		}
		candidateHash = sha256.Sum256(append(data, metadata...))
		if nativeApplyRetry.delay > 0 && nativeApplyRetry.hash == candidateHash && time.Now().Before(nativeApplyRetry.next) {
			s.SetToNeedRestart()
			return
		}
	}
	if err := s.restartLocked(ctx); err != nil {
		s.SetToNeedRestart()
		if errors.Is(err, errNativeConfigBatchChanged) {
			return
		}
		if nativeApplyRetry.hash != candidateHash {
			nativeApplyRetry.delay = 0
		}
		nativeApplyRetry.delay = min(max(nativeApplyRetry.delay*2, 5*time.Second), time.Minute)
		nativeApplyRetry.hash = candidateHash
		nativeApplyRetry.next = time.Now().Add(nativeApplyRetry.delay)
		s.process().SetError(err)
		logger.Errorf("apply native core configuration failed; retry in %s: %v", nativeApplyRetry.delay, err)
		return
	}
	nativeApplyRetry.delay = 0
}
