package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func stage3SQLite(t *testing.T, path string) *gorm.DB {
	t.Helper()
	g, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=10000"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { closeGorm(g) })
	return g
}

func stage3Legacy(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "v398.db")
	if err := RestoreSQLite("testdata/v3.9.8.sql", p); err != nil {
		t.Fatal(err)
	}
	return p
}

// Read actual stored columns, including credential fields hidden from API JSON.
func stage3Rows(t *testing.T, g *gorm.DB) map[string][]map[string]any {
	t.Helper()
	result := map[string][]map[string]any{}
	for _, m := range migrationModels() {
		s := &gorm.Statement{DB: g}
		if err := s.Parse(m); err != nil {
			t.Fatal(err)
		}
		if !g.Migrator().HasTable(m) {
			continue
		}
		var rows []map[string]any
		if err := g.Table(s.Table).Order("1").Find(&rows).Error; err != nil {
			t.Fatal(err)
		}
		result[s.Table] = rows
	}
	return result
}

func TestStage3LegacyUpgrade(t *testing.T) {
	t.Setenv("XUI_DB_TYPE", "sqlite")
	path := stage3Legacy(t)
	g := stage3SQLite(t, path)
	before := stage3Rows(t, g)
	closeGorm(g)
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	after := stage3Rows(t, GetDB())
	for table, rows := range before {
		if len(rows) != len(after[table]) && table != "history_of_seeders" {
			t.Fatalf("row loss %s", table)
		}
		for i, row := range rows {
			for col, v := range row {
				got := after[table][i][col]
				if reflect.DeepEqual(v, got) {
					continue
				}
				if table == "inbounds" && col == "settings" && strings.EqualFold(fmt.Sprint(row["protocol"]), "shadowtls") {
					var a, b map[string]any
					if json.Unmarshal([]byte(fmt.Sprint(v)), &a) != nil || json.Unmarshal([]byte(fmt.Sprint(got)), &b) != nil {
						t.Fatal("invalid legacy settings")
					}
					if b["innerKey"] == nil {
						t.Fatal("no ShadowTLS innerKey")
					}
					delete(b, "innerKey")
					if reflect.DeepEqual(a, b) {
						continue
					}
				}
				t.Fatalf("legacy value changed %s[%d].%s: %v -> %v", table, i, col, v, got)
			}
		}
	}
	if len(after["clients"]) != 51 || len(after["client_inbounds"]) != 51 || len(after["inbounds"]) != 22 {
		t.Fatal("fixture coverage lost")
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	if err := InitDB(path); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, stage3Rows(t, GetDB())) {
		t.Fatal("second opening changes data")
	}
	var fk []map[string]any
	if err := GetDB().Raw("PRAGMA foreign_key_check").Scan(&fk).Error; err != nil || len(fk) != 0 {
		t.Fatalf("FK violations: %v %v", fk, err)
	}
	if err := ValidateSQLiteDB(path); err != nil {
		t.Fatal(err)
	}
}

func TestStage3CopyRollback(t *testing.T) {
	src := stage3SQLite(t, filepath.Join(t.TempDir(), "src.db"))
	dst := stage3SQLite(t, filepath.Join(t.TempDir(), "dst.db"))
	for _, m := range migrationModels() {
		if err := src.AutoMigrate(m); err != nil {
			t.Fatal(err)
		}
	}
	if err := src.Exec("INSERT INTO users (id,username,password) VALUES(1,'source','secret')").Error; err != nil {
		t.Fatal(err)
	}
	if err := dst.AutoMigrate(migrationModels()[0]); err != nil {
		t.Fatal(err)
	}
	if err := dst.Exec("INSERT INTO users (id,username,password) VALUES(2,'destination','keep')").Error; err != nil {
		t.Fatal(err)
	}
	if err := src.Exec("DROP TABLE core_traffic_checkpoints").Error; err != nil {
		t.Fatal(err)
	}
	if err := copyAllModels(src, dst); err == nil {
		t.Fatal("broken source accepted")
	}
	var count int64
	dst.Table("users").Count(&count)
	if count != 1 {
		t.Fatalf("partial copy committed: %d users", count)
	}
	if dst.Migrator().HasTable("settings") {
		t.Fatal("schema partially committed")
	}
}

func TestStage3SQLiteWriteFailures(t *testing.T) {
	for _, kind := range []string{"busy", "full"} {
		t.Run(kind, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "failure.db")
			raw, err := sql.Open("sqlite3", p+"?_busy_timeout=20")
			if err != nil {
				t.Fatal(err)
			}
			defer raw.Close()
			raw.SetMaxOpenConns(1)
			if _, err := raw.Exec("CREATE TABLE t(id INTEGER PRIMARY KEY, value BLOB); INSERT INTO t VALUES(1,X'01')"); err != nil {
				t.Fatal(err)
			}
			if kind == "busy" {
				other, err := sql.Open("sqlite3", p)
				if err != nil {
					t.Fatal(err)
				}
				defer other.Close()
				if _, err := other.Exec("BEGIN IMMEDIATE"); err != nil {
					t.Fatal(err)
				}
				defer other.Exec("ROLLBACK")
				_, err = raw.Exec("UPDATE t SET value=X'02' WHERE id=1")
				if err == nil || !strings.Contains(err.Error(), "locked") {
					t.Fatalf("busy not surfaced: %v", err)
				}
			} else {
				var pages int
				if err := raw.QueryRow("PRAGMA page_count").Scan(&pages); err != nil {
					t.Fatal(err)
				}
				if _, err := raw.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", pages)); err != nil {
					t.Fatal(err)
				}
				tx, err := raw.BeginTx(context.Background(), nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec("UPDATE t SET value=X'02' WHERE id=1"); err != nil {
					t.Fatal(err)
				}
				if _, err := tx.Exec("INSERT INTO t VALUES(2,zeroblob(1048576))"); err == nil || !strings.Contains(err.Error(), "full") {
					t.Fatalf("disk-full not surfaced: %v", err)
				}
				_ = tx.Rollback()
			}
			var value string
			if err := raw.QueryRow("SELECT hex(value) FROM t WHERE id=1").Scan(&value); err != nil || value != "01" {
				t.Fatalf("partial write persisted: %s %v", value, err)
			}
		})
	}
}

func TestStage3DumpConcurrentSnapshot(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.db")
	raw, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_busy_timeout=10000")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err := raw.Exec("CREATE TABLE a(v INTEGER);CREATE TABLE b(v INTEGER);INSERT INTO a VALUES(0);INSERT INTO b VALUES(0); CREATE TABLE payload(v BLOB); INSERT INTO payload VALUES(zeroblob(4194304))"); err != nil {
		t.Fatal(err)
	}
	stop := make(chan struct{})
	done := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		for n := 1; ; n++ {
			select {
			case <-stop:
				done <- nil
				return
			default:
			}
			_, err := raw.Exec(fmt.Sprintf("BEGIN;UPDATE a SET v=%d;UPDATE b SET v=%d;COMMIT", n, n))
			if err != nil {
				done <- err
				return
			}
			if n == 1 {
				close(started)
			}
		}
	}()
	defer func() {
		close(stop)
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	<-started
	for i := range 3 {
		dump := filepath.Join(dir, fmt.Sprintf("%d.sql", i))
		dst := filepath.Join(dir, fmt.Sprintf("%d.db", i))
		if err := DumpSQLite(path, dump); err != nil {
			t.Fatal(err)
		}
		if err := RestoreSQLite(dump, dst); err != nil {
			t.Fatal(err)
		}
		g := stage3SQLite(t, dst)
		var a, b int
		if err := g.Raw("SELECT v FROM a").Scan(&a).Error; err != nil {
			t.Fatal(err)
		}
		if err := g.Raw("SELECT v FROM b").Scan(&b).Error; err != nil {
			t.Fatal(err)
		}
		if a != b {
			t.Fatalf("torn snapshot %d != %d", a, b)
		}
	}
}
