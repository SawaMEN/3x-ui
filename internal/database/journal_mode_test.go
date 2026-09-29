package database

import (
	"path/filepath"
	"testing"
)

func journalModeOf(t *testing.T) string {
	t.Helper()
	var mode string
	if err := db.Raw("PRAGMA journal_mode;").Scan(&mode).Error; err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	return mode
}

func TestSqliteJournalMode(t *testing.T) {
	for _, tc := range []struct {
		name  string
		value string
		want  string
	}{
		{name: "default", want: "wal"},
		{name: "environment override", value: "delete", want: "delete"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XUI_DB_JOURNAL_MODE", tc.value)
			if err := InitDB(filepath.Join(t.TempDir(), "x-ui.db")); err != nil {
				t.Fatalf("InitDB: %v", err)
			}
			t.Cleanup(func() { _ = CloseDB() })
			if got := journalModeOf(t); got != tc.want {
				t.Fatalf("journal_mode = %q, want %s", got, tc.want)
			}
		})
	}
}
