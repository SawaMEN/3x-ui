package gateway

import (
	"errors"
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func StateForCore(core string) (State, error) {
	if core == service.CoreTypeXray {
		return GetState()
	}
	if service.IsNativeCore(core) {
		return GetSingBoxState()
	}
	return State{}, fmt.Errorf("unsupported Gateway core %q", core)
}
func EnableForCore(core string) error {
	if core == service.CoreTypeXray {
		return Enable()
	}
	if service.IsNativeCore(core) {
		return EnableSingBox()
	}
	return fmt.Errorf("unsupported Gateway core %q", core)
}
func DisableForCore(core string) error {
	if core == service.CoreTypeXray {
		return Disable()
	}
	if service.IsNativeCore(core) {
		return DisableSingBox()
	}
	return fmt.Errorf("unsupported Gateway core %q", core)
}

// Caller holds AcquireOperation and suspends interception while switching the
// runtime. The returned rollback only touches Gateway-owned template objects.
func MigrateCore(oldCore, newCore string) (func() error, error) {
	if service.IsNativeCore(oldCore) && service.IsNativeCore(newCore) {
		return func() error { return nil }, nil
	}
	return migrateCore(oldCore, newCore, StateForCore, EnableForCore, DisableForCore, RestoreForCore)
}
func migrateCore(oldCore, newCore string, state func(string) (State, error), enable, disable, restore func(string) error) (func() error, error) {
	oldState, err := state(oldCore)
	if err != nil {
		return nil, err
	}
	if !oldState.Enabled {
		return func() error { return nil }, nil
	}
	newState, err := state(newCore)
	if err != nil {
		return nil, err
	}
	if !oldState.Configured || newState.Enabled {
		return nil, fmt.Errorf("Gateway recovery/conflicting templates must be cleaned up before switching cores")
	}
	if err := enable(newCore); err != nil {
		return nil, err
	}
	if err := disable(oldCore); err != nil {
		return nil, errors.Join(err, disable(newCore), restore(oldCore))
	}
	return func() error { return errors.Join(disable(newCore), restore(oldCore)) }, nil
}
