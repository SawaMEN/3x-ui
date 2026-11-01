package database

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// Exercise both production migration directions on isolated PostgreSQL schema.
// Failed exports must preserve the existing backup, while successful exports
// must copy login sessions and routing presets exactly once.
func TestPostgresSQLiteRoundTripAndBackupPreservation(t *testing.T) {
	if os.Getenv("XUI_DB_TYPE") != "postgres" || strings.TrimSpace(os.Getenv("XUI_DB_DSN")) == "" {
		t.Skip("PostgreSQL CI database is required")
	}
	baseDSN := os.Getenv("XUI_DB_DSN")
	admin, err := gorm.Open(postgres.Open(baseDSN), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	adminSQL, err := admin.DB()
	if err != nil {
		t.Fatal(err)
	}
	// The schema cleanup must run before closing the connection pool.
	t.Cleanup(func() { _ = adminSQL.Close() })

	schema := fmt.Sprintf("xui_audit_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE").Error })
	pgDSN := baseDSN + " search_path=" + schema

	dir := t.TempDir()
	source := filepath.Join(dir, "source.db")
	src, err := gorm.Open(sqlite.Open(source), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range migrationModels() {
		if err := src.AutoMigrate(entry); err != nil {
			t.Fatalf("source AutoMigrate %T: %v", entry, err)
		}
	}
	user := model.User{Id: 1, Username: "audit", Password: "hash"}
	session := model.UserSession{UserId: 1, SessionIdHash: "audit-login-session"}
	preset := model.RoutingPreset{UserId: 1, Name: "audit-routing", Rules: "[]"}
	for _, entry := range []any{&user, &session, &preset} {
		if err := src.Create(entry).Error; err != nil {
			t.Fatalf("seed %T: %v", entry, err)
		}
	}
	srcSQL, err := src.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := srcSQL.Close(); err != nil {
		t.Fatal(err)
	}

	backup := filepath.Join(dir, "backup.db")
	previous := []byte("previous valid backup")
	if err := os.WriteFile(backup, previous, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ExportPostgresToSQLite(pgDSN, backup); err == nil {
		t.Fatal("empty PostgreSQL schema unexpectedly exported")
	}
	if got, err := os.ReadFile(backup); err != nil || !bytes.Equal(got, previous) {
		t.Fatalf("failed export overwrote existing backup: %v; %q", err, got)
	}

	if err := MigrateData(source, pgDSN); err != nil {
		t.Fatalf("SQLite to PostgreSQL: %v", err)
	}
	if err := ExportPostgresToSQLite(pgDSN, backup); err != nil {
		t.Fatalf("PostgreSQL to SQLite: %v", err)
	}
	result, err := gorm.Open(sqlite.Open(backup), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	resultSQL, err := result.DB()
	if err != nil {
		t.Fatal(err)
	}
	defer resultSQL.Close()
	var gotUser model.User
	if err := result.First(&gotUser, user.Id).Error; err != nil || gotUser.Username != user.Username {
		t.Fatalf("user missing after round trip: %+v, %v", gotUser, err)
	}
	var gotSession model.UserSession
	if err := result.First(&gotSession, session.Id).Error; err != nil || gotSession.SessionIdHash != session.SessionIdHash {
		t.Fatalf("session missing after round trip: %+v, %v", gotSession, err)
	}
	var gotPreset model.RoutingPreset
	if err := result.First(&gotPreset, preset.Id).Error; err != nil || gotPreset.Name != preset.Name || gotPreset.Rules != preset.Rules {
		t.Fatalf("routing preset lost after round trip: %+v, %v", gotPreset, err)
	}
	for _, entry := range []any{&model.User{}, &model.UserSession{}, &model.RoutingPreset{}} {
		var count int64
		if err := result.Model(entry).Count(&count).Error; err != nil || count != 1 {
			t.Fatalf("round-trip %T has %d rows: %v", entry, count, err)
		}
	}
}
