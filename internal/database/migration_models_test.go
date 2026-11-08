package database

import (
	"path/filepath"
	"reflect"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestMigrationModelsMatchPanelModels(t *testing.T) {
	names := func(models []any) map[string]bool {
		set := make(map[string]bool, len(models))
		for _, m := range models {
			set[reflect.TypeOf(m).Elem().Name()] = true
		}
		return set
	}
	panel := names(allModels())
	migration := names(migrationModels())

	for name := range panel {
		if !migration[name] {
			t.Errorf("model %s is in allModels but missing from migrationModels: cross-db migration silently drops its rows", name)
		}
	}
	for name := range migration {
		if !panel[name] {
			t.Errorf("model %s is in migrationModels but missing from allModels: its table never exists on a live panel", name)
		}
	}
}

func TestMigrationModelsHaveNoDuplicates(t *testing.T) {
	seen := make(map[string]struct{}, len(migrationModels()))
	for _, entry := range migrationModels() {
		name := reflect.TypeOf(entry).Elem().Name()
		if _, found := seen[name]; found {
			t.Fatalf("migrationModels includes %s more than once: migration copies each row twice", name)
		}
		seen[name] = struct{}{}
	}
}

func TestCopyAllModelsPreservesSessionsAndRoutingPresets(t *testing.T) {
	openSQLite := func(name string) *gorm.DB {
		t.Helper()
		db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), name)), &gorm.Config{})
		if err != nil {
			t.Fatalf("open %s: %v", name, err)
		}
		raw, err := db.DB()
		if err != nil {
			t.Fatalf("get %s connection: %v", name, err)
		}
		t.Cleanup(func() { _ = raw.Close() })
		return db
	}

	src := openSQLite("source.db")
	dst := openSQLite("destination.db")
	for _, mdl := range migrationModels() {
		if err := src.AutoMigrate(mdl); err != nil {
			t.Fatalf("prepare source %T: %v", mdl, err)
		}
	}

	wantSession := model.UserSession{UserId: 1, SessionIdHash: "audit-session-hash"}
	if err := src.Create(&wantSession).Error; err != nil {
		t.Fatalf("seed session: %v", err)
	}
	wantPreset := model.RoutingPreset{UserId: 1, Name: "audit-route", Rules: "[]"}
	if err := src.Create(&wantPreset).Error; err != nil {
		t.Fatalf("seed routing preset: %v", err)
	}
	if err := copyAllModels(src, dst); err != nil {
		t.Fatalf("copy all models: %v", err)
	}

	var session model.UserSession
	if err := dst.First(&session, wantSession.Id).Error; err != nil {
		t.Fatalf("session missing: %v", err)
	}
	if session.SessionIdHash != wantSession.SessionIdHash {
		t.Fatalf("session hash = %q, want %q", session.SessionIdHash, wantSession.SessionIdHash)
	}

	var preset model.RoutingPreset
	if err := dst.First(&preset, wantPreset.Id).Error; err != nil {
		t.Fatalf("routing preset missing: %v", err)
	}
	if preset.Name != wantPreset.Name || preset.Rules != wantPreset.Rules {
		t.Fatalf("routing preset changed: got %+v, want %+v", preset, wantPreset)
	}
}
