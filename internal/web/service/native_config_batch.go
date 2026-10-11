package service

import (
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
