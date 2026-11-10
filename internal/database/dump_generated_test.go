package database

import (
	"database/sql"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestDumpRestoreGeneratedColumns(t *testing.T) {
	for _, mode := range []string{"VIRTUAL", "STORED"} {
		t.Run(mode, func(t *testing.T) {
			source := filepath.Join(t.TempDir(), "source.db")
			conn, err := sql.Open("sqlite3", source)
			if err != nil {
				t.Fatal(err)
			}
			_, err = conn.Exec(`CREATE TABLE metrics (value INTEGER, "computed""column" INTEGER GENERATED ALWAYS AS (value*2) ` + mode + `); INSERT INTO metrics(value) VALUES(21)`)
			_ = conn.Close()
			if err != nil {
				t.Fatal(err)
			}
			dump := filepath.Join(t.TempDir(), "dump.sql")
			if err = DumpSQLite(source, dump); err != nil {
				t.Fatal(err)
			}
			restored := filepath.Join(t.TempDir(), "restored.db")
			if err = RestoreSQLite(dump, restored); err != nil {
				t.Fatalf("restore generated column: %v", err)
			}
			out, err := sql.Open("sqlite3", restored)
			if err != nil {
				t.Fatal(err)
			}
			defer out.Close()
			var value, computed int
			if err = out.QueryRow(`SELECT value,"computed""column" FROM metrics`).Scan(&value, &computed); err != nil {
				t.Fatal(err)
			}
			if value != 21 || computed != 42 {
				t.Fatalf("restored values=%d,%d", value, computed)
			}
		})
	}
}

func TestDumpRestoreInfiniteReals(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.db")
	conn, err := sql.Open("sqlite3", source)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(`CREATE TABLE metrics(value REAL); INSERT INTO metrics VALUES(1e999),(-1e999)`)
	_ = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	data, err := DumpSQLiteToBytes(source)
	if err != nil {
		t.Fatal(err)
	}
	dump := filepath.Join(t.TempDir(), "dump.sql")
	if err = os.WriteFile(dump, data, 0o600); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(t.TempDir(), "restored.db")
	if err = RestoreSQLite(dump, restored); err != nil {
		t.Fatal(err)
	}
	out, err := sql.Open("sqlite3", restored)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	var positive, negative float64
	if err = out.QueryRow(`SELECT max(value),min(value) FROM metrics`).Scan(&positive, &negative); err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(positive, 1) || !math.IsInf(negative, -1) {
		t.Fatalf("infinite reals changed: %v,%v", positive, negative)
	}
}
