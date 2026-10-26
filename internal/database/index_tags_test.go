package database

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// AutoMigrate must create the hot-path indexes used by client filters, traffic
// lookups and settings reads. GORM also creates missing indexes on upgrade.
func TestAutoMigrateCreatesHotPathIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.ClientRecord{},
		&coremodel.ClientTraffic{},
		&model.ClientGlobalTraffic{},
		&model.Setting{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}

	cases := []struct {
		model any
		index string
	}{
		{&model.ClientRecord{}, "idx_client_record_group"},
		{&coremodel.ClientTraffic{}, "idx_client_traffics_inbound"},
		{&coremodel.ClientTraffic{}, "idx_client_traffics_renew"},
		{&model.ClientGlobalTraffic{}, "idx_client_global_email"},
		{&model.Setting{}, "idx_settings_key"},
	}
	for _, c := range cases {
		if !db.Migrator().HasIndex(c.model, c.index) {
			t.Errorf("expected index %q to exist after AutoMigrate", c.index)
		}
	}
}
