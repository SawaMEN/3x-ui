package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/sudoku"
)

// Serialize reconciliation with component replacement/removal, including the
// periodic job. A successful update means the enabled listeners are running.
func ReconcileSudoku(ctx context.Context) error {
	protocolInstallMu.Lock()
	defer protocolInstallMu.Unlock()
	desired, err := DesiredSudokuInstances()
	if err != nil {
		return err
	}
	sudoku.GetManager().Reconcile(ctx, desired)
	return nil
}

func UpdateSudoku(ctx context.Context) error {
	return installSudoku(ctx, sudoku.Update)
}

func installSudoku(ctx context.Context, install func(context.Context) error) error {
	protocolInstallMu.Lock()
	defer protocolInstallMu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	dir := config.GetBinFolderPath()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var restores []func() error
	var cleanups []func()
	defer func() {
		for _, cleanup := range cleanups {
			cleanup()
		}
	}()
	for _, path := range []string{sudoku.GetBinaryPath(dir), filepath.Join(dir, "sudoku.version")} {
		restore, cleanup, err := snapshotSudokuFile(path)
		if err != nil {
			return err
		}
		restores = append(restores, restore)
		cleanups = append(cleanups, cleanup)
	}
	apply := func(ctx context.Context) error {
		desired, err := desiredSudokuInstances(true)
		if err != nil {
			return err
		}
		sudoku.GetManager().Reconcile(ctx, desired)
		return sudoku.GetManager().Check(desired)
	}
	previousBinary, _ := os.Stat(sudoku.GetBinaryPath(dir))
	err := install(ctx)
	if err != nil {
		currentBinary, _ := os.Stat(sudoku.GetBinaryPath(dir))
		if (previousBinary == nil && currentBinary == nil) || (previousBinary != nil && currentBinary != nil && os.SameFile(previousBinary, currentBinary)) {
			// A download/validation failure before replacement must not drop active sessions.
			return err
		}
	}
	if err == nil {
		err = ctx.Err()
	}
	if err == nil {
		err = apply(ctx)
	}
	if err == nil {
		return nil
	}
	// A rejected build may have started some listeners. Stop them before
	// restoring the previous executable, then recover using an independent
	// context even if the HTTP request was canceled.
	sudoku.GetManager().StopAll()
	var restoreErr error
	for _, restore := range restores {
		restoreErr = errors.Join(restoreErr, restore())
	}
	if restoreErr != nil {
		return errors.Join(err, fmt.Errorf("restore Sudoku installation: %w", restoreErr))
	}
	recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return errors.Join(err, apply(recovery))
}

func snapshotSudokuFile(path string) (restore func() error, cleanup func(), err error) {
	source, err := os.Open(path)
	if os.IsNotExist(err) {
		return func() error { return removeComponentFile(path) }, func() {}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return nil, nil, err
	}
	target, err := os.CreateTemp(filepath.Dir(path), ".sudoku-previous-*")
	if err != nil {
		return nil, nil, err
	}
	backup := target.Name()
	cleanup = func() { _ = os.Remove(backup) }
	_, copyErr := io.Copy(target, source)
	err = errors.Join(copyErr, target.Close(), os.Chmod(backup, info.Mode().Perm()))
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return func() error { return os.Rename(backup, path) }, cleanup, nil
}
