package naiveproxy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// Ensure synchronously installs and reconciles an explicitly requested standalone server.
func Ensure(ctx context.Context) error {
	needed, err := standaloneServerNeeded()
	if err != nil {
		return err
	}
	if !needed {
		return reconcileStandalone(ctx)
	}
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		return fmt.Errorf("official Caddy-Naive server release supports linux/amd64 only; current platform is %s/%s", runtime.GOOS, runtime.GOARCH)
	}
	if _, err := os.Stat(BinaryPath()); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if _, err := Update(ctx); err != nil {
			return err
		}
	}
	return reconcileStandalone(ctx)
}

func standaloneServerNeeded() (bool, error) {
	db := database.GetDB()
	if db == nil {
		return false, nil
	}
	coreType := "xray"
	var setting struct{ Value string }
	if err := db.Table("settings").Select("value").Where("key = ?", "coreType").Take(&setting).Error; err == nil && setting.Value != "" {
		coreType = setting.Value
	}
	if coreType != "xray" {
		return false, nil
	}
	var count int64
	if err := db.Model(&model.Inbound{}).
		Where("protocol = ? AND enable = ? AND node_id IS NULL", model.NaiveProxy, true).
		Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}
