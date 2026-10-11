package runtime

import (
	"context"
	"errors"
	"sync"
)

// ConfigBatchRuntime is optional: remote APIs retain their per-client contract.
// A scope encloses the DB mutations and all post-commit local runtime calls.
type ConfigBatchRuntime interface {
	BeginConfigBatch() func() error
	ConfigBatchActive() bool
}

var errNativeApplyNotConfigured = errors.New("native core configuration apply is not configured")

func (l *Local) BeginConfigBatch() func() error {
	if !l.isNative() {
		return func() error { return nil }
	}
	l.batchMu.Lock()
	l.batches++
	l.batchMu.Unlock()
	var once sync.Once
	var err error
	return func() error {
		once.Do(func() {
			l.batchMu.Lock()
			l.batches--
			apply := l.batches == 0 && l.batchDirty
			if apply {
				l.batchDirty = false
			}
			l.batchMu.Unlock()
			if apply {
				err = l.requestNativeApply(context.Background())
			}
		})
		return err
	}
}

func (l *Local) ConfigBatchActive() bool {
	l.batchMu.Lock()
	defer l.batchMu.Unlock()
	return l.batches > 0
}

func (l *Local) requestNativeApply(ctx context.Context) error {
	if l.deps.RequestConfigApply != nil {
		return l.deps.RequestConfigApply(ctx)
	}
	if l.deps.RestartCore != nil {
		return l.deps.RestartCore(ctx)
	}
	return errNativeApplyNotConfigured
}

func (l *Local) UsesNativeConfig() bool { return l.isNative() }
