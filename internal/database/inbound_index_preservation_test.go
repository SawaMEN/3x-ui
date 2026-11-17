package database

import (
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestLegacyPortMigrationPreservesCustomIndexAndTrigger(t *testing.T) {
	oldDB := db
	local, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db = local
	t.Cleanup(func() { raw, _ := local.DB(); _ = raw.Close(); db = oldDB })
	for _, statement := range []string{
		legacyInboundsDDL,
		`CREATE INDEX "custom_remark" ON inbounds(remark)`,
		`CREATE TABLE inbound_audit(value TEXT)`,
		`CREATE TRIGGER audit_inbound AFTER UPDATE OF remark ON inbounds BEGIN INSERT INTO inbound_audit VALUES(new.remark); END`,
		`INSERT INTO inbounds(port,tag,remark) VALUES(443,'test','old')`,
	} {
		if err = local.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = dropLegacyInboundPortUnique(); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err = local.Raw(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name=?`, `custom_remark`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("custom index lost: count=%d err=%v", count, err)
	}
	if err = local.Exec(`UPDATE inbounds SET remark='new' WHERE tag='test'`).Error; err != nil {
		t.Fatal(err)
	}
	if err = local.Raw(`SELECT count(*) FROM inbound_audit WHERE value='new'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("trigger lost or changed: count=%d err=%v", count, err)
	}
	if err = local.Exec(`INSERT INTO inbounds(port,tag) VALUES(443,'another')`).Error; err != nil {
		t.Fatalf("legacy port uniqueness retained: %v", err)
	}
}

func TestLegacyPortMigrationQuotesExplicitIndexNames(t *testing.T) {
	oldDB := db
	local, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db = local
	t.Cleanup(func() { raw, _ := local.DB(); _ = raw.Close(); db = oldDB })
	if err = local.Exec(`CREATE TABLE inbounds(id INTEGER PRIMARY KEY,port INTEGER,tag TEXT); CREATE UNIQUE INDEX "legacy""port" ON inbounds(port)`).Error; err != nil {
		t.Fatal(err)
	}
	if err = dropLegacyInboundPortUnique(); err != nil {
		t.Fatal(err)
	}
	if err = local.Exec(`INSERT INTO inbounds(port,tag) VALUES(443,'a'),(443,'b')`).Error; err != nil {
		t.Fatal(err)
	}
}

func TestLegacyPortMigrationPreservesDependentReferences(t *testing.T) {
	oldDB := db
	local, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db = local
	t.Cleanup(func() { raw, _ := local.DB(); _ = raw.Close(); db = oldDB })
	for _, statement := range []string{
		legacyInboundsDDL,
		`INSERT INTO inbounds(port,tag,remark) VALUES(443,'test','keep')`,
		`CREATE VIEW inbound_view AS SELECT remark FROM inbounds`,
		`CREATE TABLE child(inbound_id INTEGER REFERENCES inbounds(id))`,
	} {
		if err = local.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err = dropLegacyInboundPortUnique(); err != nil {
		t.Fatal(err)
	}
	var remark string
	if err = local.Raw(`SELECT remark FROM inbound_view`).Scan(&remark).Error; err != nil || remark != "keep" {
		t.Fatalf("view reference corrupted: %q %v", remark, err)
	}
	var references []struct{ Table string }
	if err = local.Raw(`PRAGMA foreign_key_list(child)`).Scan(&references).Error; err != nil || len(references) != 1 || references[0].Table != "inbounds" {
		t.Fatalf("foreign key reference corrupted: %+v %v", references, err)
	}
	var mode int
	if err = local.Raw(`PRAGMA legacy_alter_table`).Scan(&mode).Error; err != nil || mode != 0 {
		t.Fatalf("connection pragma leaked: %d %v", mode, err)
	}
}

func TestLegacyPortMigrationRollbackRestoresSchemaAndPragma(t *testing.T) {
	oldDB := db
	local, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db = local
	t.Cleanup(func() { raw, _ := local.DB(); _ = raw.Close(); db = oldDB })
	for _, statement := range []string{
		legacyInboundsDDL,
		`ALTER TABLE inbounds ADD COLUMN extension TEXT`,
		`CREATE INDEX extension_index ON inbounds(extension)`,
		`INSERT INTO inbounds(port,tag,extension) VALUES(443,'keep','original')`,
	} {
		if err = local.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	// A foreign extension cannot be replayed against the panel-only model.
	// Reject that upgrade atomically rather than silently losing its schema.
	if err = dropLegacyInboundPortUnique(); err == nil {
		t.Fatal("unsupported extension schema was silently discarded")
	}
	var value string
	if err = local.Raw(`SELECT extension FROM inbounds WHERE tag='keep'`).Scan(&value).Error; err != nil || value != "original" {
		t.Fatalf("failed upgrade lost original data: %q %v", value, err)
	}
	var mode int
	if err = local.Raw(`PRAGMA legacy_alter_table`).Scan(&mode).Error; err != nil || mode != 0 {
		t.Fatalf("failed upgrade leaked connection pragma: %d %v", mode, err)
	}
	var count int64
	if err = local.Raw(`SELECT count(*) FROM sqlite_master WHERE type='index' AND name='extension_index'`).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("failed upgrade lost original index: %d %v", count, err)
	}
}
