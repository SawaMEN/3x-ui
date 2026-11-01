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
	l.batchMu.Lock()
	l.batches++
	l.batchVersion++
	l.batchMu.Unlock()
	var once sync.Once
	var err error
	return func() error {
		once.Do(func() {
			l.batchMu.Lock()
			l.batches--
			l.batchVersion++
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

func (l *Local) UsesNativeConfig() bool { return true }

// ConfigBatchVersion changes even when a scope opens and closes during validation.
func (l *Local) ConfigBatchVersion() uint64 {
	l.batchMu.Lock()
	defer l.batchMu.Unlock()
	return l.batchVersion
}
