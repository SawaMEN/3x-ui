package database

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStage3DumpPreservesStorageClasses(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.db")
	dump := filepath.Join(dir, "dump.sql")
	dst := filepath.Join(dir, "dst.db")
	raw, err := sql.Open("sqlite3", src)
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	_, err = raw.Exec(`CREATE TABLE "quoted""table" (b BLOB, n TEXT, z BLOB); INSERT INTO "quoted""table" VALUES (X'616263',CAST(X'610062' AS TEXT),X'');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := DumpSQLite(src, dump); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSQLite(dump, dst); err != nil {
		t.Fatal(err)
	}
	restored, err := sql.Open("sqlite3", dst)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var b, n, z, types string
	if err := restored.QueryRow(`SELECT hex(b),hex(n),hex(z),typeof(b)||','||typeof(n)||','||typeof(z) FROM "quoted""table"`).Scan(&b, &n, &z, &types); err != nil {
		t.Fatal(err)
	}
	if b != "616263" || n != "610062" || z != "" || types != "blob,text,blob" {
		t.Fatalf("storage changed: %q %q %q %q", b, n, z, types)
	}
	info, err := os.Stat(dst)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("restore permissions: %v %v", info, err)
	}
}

func TestStage3RestoreRejectsUncommittedDump(t *testing.T) {
	dir := t.TempDir()
	dump := filepath.Join(dir, "bad.sql")
	dst := filepath.Join(dir, "dst.db")
	if err := os.WriteFile(dump, []byte("BEGIN; CREATE TABLE t(x); INSERT INTO t VALUES(1);"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := RestoreSQLite(dump, dst); err == nil || !strings.Contains(err.Error(), "not committed") {
		t.Fatalf("truncated dump accepted: %v", err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatalf("partial destination remains: %v", err)
	}
}

func TestStage3ExportFailurePreservesDestination(t *testing.T) {
	dst := filepath.Join(t.TempDir(), "existing.db")
	if err := os.WriteFile(dst, []byte("previous-backup"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ExportPostgresToSQLite("host=127.0.0.1 port=1 user=unused dbname=unused connect_timeout=1 sslmode=disable", dst); err == nil {
		t.Fatal("unreachable postgres accepted")
	}
	data, err := os.ReadFile(dst)
	if err != nil || string(data) != "previous-backup" {
		t.Fatalf("previous backup lost: %q %v", data, err)
	}
}

func BenchmarkDumpTableData(b *testing.B) {
	path := filepath.Join(b.TempDir(), "bench.db")
	raw, err := sql.Open("sqlite3", path)
	if err != nil {
		b.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec(`CREATE TABLE t(id INTEGER, value TEXT); WITH RECURSIVE n(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM n WHERE x<10000) INSERT INTO t SELECT x, 'synthetic-value' FROM n`); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var out strings.Builder
		if err := dumpTableData(raw, "t", &out); err != nil {
			b.Fatal(err)
		}
	}
}

func TestStage3ValidateRejectsIncompleteFiles(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("SQLite format 3\x00"), []byte("not a database")} {
		p := filepath.Join(t.TempDir(), "bad.db")
		if err := os.WriteFile(p, data, 0600); err != nil {
			t.Fatal(err)
		}
		if err := ValidateSQLiteDB(p); err == nil {
			t.Fatal("incomplete/corrupt database accepted")
		}
	}
}
