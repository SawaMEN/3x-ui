package coremigration

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// completed reads the current marker without schema migration or seeding.
func storedCoreSettings(ctx context.Context) (map[string]string, error) {
	var dialect gorm.Dialector
	if config.GetDBKind() == "postgres" {
		dialect = postgres.Open(config.GetDBDSN())
	} else {
		absolute, err := filepath.Abs(config.GetDBPath())
		if err != nil {
			return nil, err
		}
		source := (&url.URL{Scheme: "file", Path: absolute}).String() + "?mode=ro"
		dialect = sqlite.Open(source)
	}
	db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	defer sqlDB.Close()
	var rows []struct{ Key, Value string }
	values := map[string]string{}
	if db.Migrator().HasTable("settings") {
		if err := db.WithContext(ctx).Table("settings").Select("key, value").Where("key IN ?", []string{"coreType", "ownerCoreMigration", "xrayTemplateConfig"}).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, r := range rows {
			values[r.Key] = r.Value
		}
	}
	if db.Migrator().HasTable("inbounds") {
		var count int64
		if err := db.WithContext(ctx).Table("inbounds").Count(&count).Error; err != nil {
			return nil, err
		}
		if count > 0 {
			values["hasInbounds"] = "true"
		}
	}
	return values, nil
}

func completed(ctx context.Context) (bool, error) {
	values, err := storedCoreSettings(ctx)
	if err != nil {
		return false, err
	}
	return values["coreType"] == "sing-box" && values["ownerCoreMigration"] == hiddify.TargetRevision, nil
}

func checkStoredCore() error {
	if config.GetDBKind() != "postgres" {
		if _, err := os.Stat(config.GetDBPath()); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	values, err := storedCoreSettings(context.Background())
	if err != nil {
		return fmt.Errorf("read core settings before startup: %w", err)
	}
	if values["coreType"] == "" && values["hasInbounds"] == "" && values["xrayTemplateConfig"] == "" {
		return nil
	}
	if values["coreType"] == "xray" || values["coreType"] == "" {
		return fmt.Errorf("legacy core configuration requires offline migration; stop the panel and run x-ui core-migrate with a recovery backup")
	}
	if values["coreType"] != "sing-box" && values["coreType"] != "hiddify-core" {
		return fmt.Errorf("unsupported persisted core %q", values["coreType"])
	}
	return nil
}

// CheckPanelStartup must run while holding the panel lease, before InitDB.
// An interrupted offline transition must be recovered before serving requests.
func CheckPanelStartup() error {
	data, err := os.ReadFile(pendingPath())
	if os.IsNotExist(err) {
		return checkStoredCore()
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("unfinished core migration; keep panel stopped and restore snapshot %s", string(data))
}
