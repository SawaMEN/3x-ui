package naiveproxy

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

var automaticLifecycleOnce sync.Once

// StartAutomaticLifecycle continuously converges standalone Naive state to the
// panel database. The official Caddy-Naive binary is installed lazily only when
// Xray is active and at least one local enabled Naive inbound exists.
func StartAutomaticLifecycle() {
	automaticLifecycleOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(reconcileInterval)
			defer ticker.Stop()
			for {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				if err := Ensure(ctx); err != nil && database.GetDB() != nil {
					logger.Warning("NaiveProxy automatic lifecycle failed:", err)
				}
				cancel()
				<-ticker.C
			}
		}()
	})
}

// Ensure is the synchronous form used after explicit updates and by the
// background lifecycle. It installs the server only if it is actually needed.
func Ensure(ctx context.Context) error {
	needed, err := standaloneServerNeeded()
	if err != nil {
		return err
	}
	if !needed {
		return Reconcile(ctx)
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
	return Reconcile(ctx)
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
