package database

import (
	"reflect"
	"testing"
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
