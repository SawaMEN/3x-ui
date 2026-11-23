package database

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestSQLiteReadSnapshotDoesNotReserveWriter(t *testing.T) {
	t.Setenv("XUI_DB_TYPE", "sqlite")
	t.Setenv("XUI_DB_JOURNAL_MODE", "WAL")
	t.Setenv("XUI_DB_MAX_OPEN_CONNS", "1")
	if err := InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	if err := db.Exec("CREATE TABLE snapshot_probe (value INTEGER NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("INSERT INTO snapshot_probe VALUES (1)").Error; err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	writer := db.WithContext(ctx).Begin()
	if writer.Error != nil {
		t.Fatal(writer.Error)
	}
	defer writer.Rollback()
	if err := writer.Exec("UPDATE snapshot_probe SET value = 2").Error; err != nil {
		t.Fatal(err)
	}

	err := ReadSnapshot(db.WithContext(ctx), func(tx *gorm.DB) error {
		var first, second int
		if err := tx.Raw("SELECT value FROM snapshot_probe").Scan(&first).Error; err != nil {
			return err
		}
		if first != 1 {
			t.Errorf("read uncommitted value: %d", first)
		}
		// Commit the writer while the read snapshot is still open. WAL readers
		// must not block writers, and both reads must see the same snapshot.
		if err := writer.Commit().Error; err != nil {
			return err
		}
		if err := tx.Raw("SELECT value FROM snapshot_probe").Scan(&second).Error; err != nil {
			return err
		}
		if second != first {
			t.Errorf("snapshot changed from %d to %d", first, second)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ReadSnapshot(db, func(tx *gorm.DB) error {
		return tx.Exec("UPDATE snapshot_probe SET value = 3").Error
	}); err == nil {
		t.Fatal("read-only SQLite snapshot accepted a write")
	}
	var got int
	if err := db.Raw("SELECT value FROM snapshot_probe").Scan(&got).Error; err != nil || got != 2 {
		t.Fatalf("writer value = %d, err = %v", got, err)
	}
}
