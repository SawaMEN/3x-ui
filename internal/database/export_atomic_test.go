package database

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestExportPostgresToSQLitePreservesPreviousBackupOnSourceFailure(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "panel.db")
	previous := []byte("previous successful export must survive connection errors")
	if err := os.WriteFile(destination, previous, 0o600); err != nil {
		t.Fatal(err)
	}

	// Port 1 on loopback is not a PostgreSQL instance. Connection failure must
	// not remove, truncate or replace an existing user backup.
	err := ExportPostgresToSQLite(
		"host=127.0.0.1 port=1 user=invalid password=invalid dbname=invalid sslmode=disable connect_timeout=1",
		destination,
	)
	if err == nil {
		t.Fatal("expected PostgreSQL connection failure")
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("previous backup was deleted: %v", err)
	}
	if !bytes.Equal(got, previous) {
		t.Fatalf("failed export corrupted previous backup: got %q", got)
	}
	leftovers, err := filepath.Glob(filepath.Join(dir, ".x-ui-export-*.db"))
	if err != nil {
		t.Fatal(err)
	}
	if len(leftovers) > 0 {
		t.Fatalf("failed export left temporary SQLite files: %v", leftovers)
	}
}
