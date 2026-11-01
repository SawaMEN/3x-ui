package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Stage and check a replacement before interrupting the running engine. The
// previous executable remains available until the new process has started.
func UpdateStandaloneCore(ctx context.Context, engine, version string) (string, error) {
	return updateStandaloneCore(ctx, engine, func(target string) (string, error) { return installStandaloneAt(ctx, engine, version, target) })
}

func updateStandaloneCore(ctx context.Context, engine string, install func(string) (string, error)) (updatedVersion string, resultErr error) {
	if engine != CoreTypeXray && engine != CoreTypeSingBoxUpstream {
		return "", fmt.Errorf("the bundled kernel is updated with the panel")
	}
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	finishSwitch := BeginCoreSwitch()
	defer finishSwitch()
	svc := NativeCore(engine)
	target := svc.BinaryPath()
	if !svc.Installed() {
		return "", fmt.Errorf("install the core first")
	}
	if err := cleanupStandaloneCoreArtifacts(engine, target, false); err != nil {
		return "", err
	}
	staged, err := os.CreateTemp(filepath.Dir(target), ".core-"+engine+"-update-*")
	if err != nil {
		return "", err
	}
	_ = staged.Close()
	defer os.Remove(staged.Name())
	version, err := install(staged.Name())
	if err != nil {
		return "", err
	}
	wasRunning := svc.IsRunning()
	if wasRunning {
		check := singbox.NewProcessWithOptions(svc.ProcessConfigPath(), standaloneProfile(engine, staged.Name()), true)
		if err := check.Validate(ctx); err != nil {
			return "", err
		}
		_ = svc.pollStandaloneTraffic(ctx)
	}
	backup, err := os.CreateTemp(filepath.Dir(target), ".core-"+engine+"-backup-*")
	if err != nil {
		return "", err
	}
	_ = backup.Close()
	keepBackup := false
	defer func() {
		if !keepBackup {
			_ = os.Remove(backup.Name())
		}
	}()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	// Keep gateway policy suspended only over the actual process replacement.
	finishGateway := func() error { return nil }
	if wasRunning && IsNativeCore(engine) {
		finishGateway, err = svc.pauseGateway(ctx)
		if err != nil {
			return "", err
		}
		defer func() { resultErr = errors.Join(resultErr, finishGateway()) }()
	}
	if wasRunning {
		if err := svc.process().Stop(); err != nil {
			return "", err
		}
	}
	restore := func(cause error) error {
		_ = svc.process().Stop()
		rollbackErr := os.Rename(backup.Name(), target)
		if rollbackErr != nil {
			keepBackup = true
			rollbackErr = fmt.Errorf("previous core preserved at %s: %w", backup.Name(), rollbackErr)
		}
		if rollbackErr == nil && wasRunning {
			rollbackErr = svc.process().Start(context.Background())
		}
		return errors.Join(cause, rollbackErr)
	}
	if err := os.Rename(target, backup.Name()); err != nil {
		if wasRunning {
			err = errors.Join(err, svc.process().Start(context.Background()))
		}
		return "", err
	}
	if err := os.Rename(staged.Name(), target); err != nil {
		return "", restore(err)
	}
	if wasRunning {
		if err := svc.process().Start(ctx); err != nil {
			return "", restore(err)
		}
	}
	return version, nil
}
