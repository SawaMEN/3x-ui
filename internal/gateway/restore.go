package gateway

import (
	"fmt"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// RestoreForCore repairs an interrupted disable without replacing the original
// backup or discarding template edits. The caller holds the operation lock.
func RestoreForCore(core string) error {
	operationMu.Lock()
	defer operationMu.Unlock()
	var cfg map[string]any
	var raw string
	var err error
	var exists func() (bool, error)
	var apply func(map[string]any) error
	var create func(string) error
	var save func(map[string]any) error
	var remove func() error
	switch core {
	case service.CoreTypeXray:
		cfg, raw, err = loadTemplate()
		exists, apply, create, save, remove = backupExists, applyGatewayConfig, createBackup, saveTemplate, removeBackup
	case service.CoreTypeSingBox, service.CoreTypeHiddify:
		cfg, raw, err = loadSingBoxTemplate()
		exists, apply, create, save, remove = singBoxBackupExists, applySingBoxGatewayConfig, createSingBoxBackup, saveSingBoxTemplate, removeSingBoxBackup
	default:
		return fmt.Errorf("unsupported Gateway core %q", core)
	}
	if err != nil {
		return err
	}
	backup, err := exists()
	if err != nil {
		return err
	}
	if err := apply(cfg); err != nil {
		return err
	}
	if !backup {
		if err := create(raw); err != nil {
			return err
		}
	}
	if err := save(cfg); err != nil {
		if !backup {
			if cleanup := remove(); cleanup != nil {
				return fmt.Errorf("%w; backup cleanup failed: %w", err, cleanup)
			}
		}
		return err
	}
	return nil
}
