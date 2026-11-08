package service

import (
	"errors"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

func localConfigBatch() runtime.ConfigBatchRuntime {
	manager := runtime.GetManager()
	if manager == nil {
		return nil
	}
	rt, err := manager.RuntimeFor(nil)
	if err != nil {
		return nil
	}
	batch, _ := rt.(runtime.ConfigBatchRuntime)
	return batch
}

func beginLocalConfigBatch() func() {
	batch := localConfigBatch()
	if batch == nil {
		return func() {}
	}
	finish := batch.BeginConfigBatch()
	return func() {
		if err := finish(); err != nil {
			logger.Error("queue local core configuration apply failed:", err)
			SelectedNativeCore().SetToNeedRestart()
		}
	}
}

func rtUsesNativeConfig(rt runtime.Runtime) bool {
	native, ok := rt.(interface{ UsesNativeConfig() bool })
	return ok && native.UsesNativeConfig()
}

// BeginLocalConfigBatch groups a composed operation (for example LDAP sync)
// across several client service calls. Always defer the returned function.
func BeginLocalConfigBatch() func() { return beginLocalConfigBatch() }

var errNativeConfigBatchChanged = errors.New("native configuration batch changed during preparation")

// A completed scope matters too: checking only Active can publish a snapshot
// captured halfway through a bulk edit after that edit has already finished.
func nativeConfigBatchGuard() (func() bool, error) {
	batch := localConfigBatch()
	if batch == nil {
		return func() bool { return true }, nil
	}
	versioned, ok := batch.(interface{ ConfigBatchVersion() uint64 })
	var version uint64
	if ok {
		version = versioned.ConfigBatchVersion()
	}
	if batch.ConfigBatchActive() {
		return nil, errNativeConfigBatchChanged
	}
	return func() bool {
		if batch.ConfigBatchActive() {
			return false
		}
		return !ok || versioned.ConfigBatchVersion() == version
	}, nil
}
