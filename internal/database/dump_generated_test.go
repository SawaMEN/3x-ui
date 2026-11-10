package database

import (
	"bytes"
	"database/sql"
	"errors"
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

func BenchmarkDumpSQLiteFile(b *testing.B) {
	source := filepath.Join(b.TempDir(), "source.db")
	conn, err := sql.Open("sqlite3", source)
	if err != nil {
		b.Fatal(err)
	}
	_, err = conn.Exec(`CREATE TABLE payload(id INTEGER PRIMARY KEY,value BLOB); WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<1024) INSERT INTO payload SELECT x,zeroblob(4096) FROM n`)
	_ = conn.Close()
	if err != nil {
		b.Fatal(err)
	}
	destination := filepath.Join(b.TempDir(), "backup.sql")
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if err := DumpSQLite(source, destination); err != nil {
			b.Fatal(err)
		}
	}
}

func TestStreamingDumpMatchesMemoryExportAndPropagatesWriteErrors(t *testing.T) {
	source := filepath.Join(t.TempDir(), "source.db")
	conn, err := sql.Open("sqlite3", source)
	if err != nil {
		t.Fatal(err)
	}
	_, err = conn.Exec(`CREATE TABLE payload(value TEXT); INSERT INTO payload VALUES('one'),('two')`)
	_ = conn.Close()
	if err != nil {
		t.Fatal(err)
	}
	expected, err := DumpSQLiteToBytes(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "dump.sql")
	if err = DumpSQLite(source, path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, expected) {
		t.Fatalf("file and memory formats differ: %v", err)
	}
	rejected := errors.New("synthetic output write failure")
	if err = dumpSQLiteTo(source, rejectedDumpOutput{rejected}); !errors.Is(err, rejected) {
		t.Fatalf("write failure lost: %v", err)
	}
}

type rejectedDumpOutput struct{ err error }

func (w rejectedDumpOutput) Write([]byte) (int, error) { return 0, w.err }
