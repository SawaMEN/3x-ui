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
func completed(ctx context.Context) (bool, error) {
	var dialect gorm.Dialector
	if config.GetDBKind() == "postgres" {
		dialect = postgres.Open(config.GetDBDSN())
	} else {
		absolute, err := filepath.Abs(config.GetDBPath())
		if err != nil {
			return false, err
		}
		source := (&url.URL{Scheme: "file", Path: absolute}).String() + "?mode=ro"
		dialect = sqlite.Open(source)
	}
	db, err := gorm.Open(dialect, &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return false, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return false, err
	}
	defer sqlDB.Close()
	var rows []struct{ Key, Value string }
	if err := db.WithContext(ctx).Table("settings").Select("key, value").Where("key IN ?", []string{"coreType", "ownerCoreMigration"}).Find(&rows).Error; err != nil {
		return false, err
	}
	values := map[string]string{}
	for _, r := range rows {
		values[r.Key] = r.Value
	}
	return values["coreType"] == "sing-box" && values["ownerCoreMigration"] == hiddify.TargetRevision, nil
}

// CheckPanelStartup must run while holding the panel lease, before InitDB.
// An interrupted offline transition must be recovered before serving requests.
func CheckPanelStartup() error {
	data, err := os.ReadFile(pendingPath())
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("unfinished core migration; keep panel stopped and restore snapshot %s", string(data))
}
