//go:build linux

package coremigration

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

// AcquirePanelLease excludes migration from a running panel and other migrations.
func AcquirePanelLease() (func(), error) {
	if err := os.MkdirAll(config.GetDBFolderPath(), 0o700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(config.GetDBFolderPath(), "core-migration.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("panel or core migration is running; stop x-ui before migration: %w", err)
	}
	return func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN); _ = f.Close() }, nil
}
