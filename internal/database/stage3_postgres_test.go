package database

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func stage3Postgres(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	dsn := os.Getenv("XUI_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("requires isolated XUI_TEST_PG_DSN")
	}
	admin, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("stage3_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	scoped := dsn + " search_path=" + schema
	if u, err := url.Parse(dsn); err == nil && (u.Scheme == "postgres" || u.Scheme == "postgresql") {
		q := u.Query()
		q.Set("search_path", schema)
		u.RawQuery = q.Encode()
		scoped = u.String()
	}
	g, err := gorm.Open(postgres.Open(scoped), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeGorm(g)
		if err := admin.Exec("DROP SCHEMA " + schema + " CASCADE").Error; err != nil {
			t.Error(err)
		}
		closeGorm(admin)
	})
	return g, scoped
}

func stage3Canonical(t *testing.T, g *gorm.DB) string {
	t.Helper()
	result := map[string][]map[string]any{}
	for _, m := range migrationModels() {
		s := &gorm.Statement{DB: g}
		if err := s.Parse(m); err != nil {
			t.Fatal(err)
		}
		slice := reflect.New(reflect.SliceOf(reflect.TypeOf(m)))
		if err := g.Order("1").Find(slice.Interface()).Error; err != nil {
			t.Fatal(err)
		}
		rows := make([]map[string]any, 0, slice.Elem().Len())
		for i := 0; i < slice.Elem().Len(); i++ {
			row := map[string]any{}
			for _, name := range s.Schema.DBNames {
				v, _ := s.Schema.FieldsByDBName[name].ValueOf(context.Background(), reflect.Indirect(slice.Elem().Index(i)))
				if value, ok := v.(driver.Valuer); ok {
					var err error
					v, err = value.Value()
					if err != nil {
						t.Fatal(err)
					}
				}
				row[name] = v
			}
			rows = append(rows, row)
		}
		result[s.Table] = rows
	}
	b, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func stage3PopulatedLegacy(t *testing.T) string {
	t.Helper()
	t.Setenv("XUI_DB_TYPE", "sqlite")
	p := stage3Legacy(t)
	if err := InitDB(p); err != nil {
		t.Fatal(err)
	}
	g := GetDB()
	// Populate every service table added after3.9.8, not just historical tables.
	for _, m := range migrationModels() {
		s := &gorm.Statement{DB: g}
		if err := s.Parse(m); err != nil {
			t.Fatal(err)
		}
		var n int64
		if err := g.Model(m).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			continue
		}
		row := map[string]any{}
		v := reflect.Indirect(reflect.ValueOf(m))
		for _, name := range s.Schema.DBNames {
			f := s.Schema.FieldsByDBName[name]
			value, _ := f.ValueOf(context.Background(), v)
			if f.AutoIncrement {
				value = 900001
			}

			row[name] = value
		}
		if err := g.Table(s.Table).Create(row).Error; err != nil {
			t.Fatalf("populate %s: %v", s.Table, err)
		}
	}
	if err := CloseDB(); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStage3PostgresRoundTrip(t *testing.T) {
	pg, dsn := stage3Postgres(t)
	path := stage3PopulatedLegacy(t)
	src := stage3SQLite(t, path)
	want := stage3Canonical(t, src)
	for i := range 2 {
		if err := MigrateData(path, dsn); err != nil {
			t.Fatalf("migration%d: %v", i, err)
		}
		if got := stage3Canonical(t, pg); got != want {
			t.Fatal("SQLite->Postgres changes stored model fields (including hidden credentials)")
		}
	}
	for _, m := range migrationModels() {
		s := &gorm.Statement{DB: pg}
		if err := s.Parse(m); err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := pg.Model(m).Count(&count).Error; err != nil || count == 0 {
			t.Fatalf("unpopulated model %T: %v", m, err)
		}
		if f := s.Schema.LookUpField("id"); f != nil && f.AutoIncrement {
			var ok bool
			if err := pg.Raw(`SELECT nextval(pg_get_serial_sequence(?, 'id')) > (SELECT COALESCE(MAX(id),0) FROM "`+s.Table+`")`, s.Table).Scan(&ok).Error; err != nil || !ok {
				t.Fatalf("sequence %s: %v", s.Table, err)
			}
		}
	}
	out := filepath.Join(t.TempDir(), "export.db")
	if err := ExportPostgresToSQLite(dsn, out); err != nil {
		t.Fatal(err)
	}
	restored := stage3SQLite(t, out)
	if got := stage3Canonical(t, restored); got != want {
		t.Fatal("Postgres->SQLite changes stored model fields")
	}
	if err := ValidateSQLiteDB(out); err != nil {
		t.Fatal(err)
	}
}

func TestStage3PostgresCommitAndDisconnect(t *testing.T) {
	pg, dsn := stage3Postgres(t)
	path := stage3PopulatedLegacy(t)
	if err := MigrateData(path, dsn); err != nil {
		t.Fatal(err)
	}
	want := stage3Canonical(t, pg)
	if err := pg.Exec(`CREATE FUNCTION fail_stage3_commit() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'stage3 deferred commit failure'; END $$; CREATE CONSTRAINT TRIGGER fail_stage3_commit AFTER INSERT ON users DEFERRABLE INITIALLY DEFERRED FOR EACH ROW EXECUTE FUNCTION fail_stage3_commit()`).Error; err != nil {
		t.Fatal(err)
	}
	if err := MigrateData(path, dsn); err == nil {
		t.Fatal("commit failure reported success")
	}
	if stage3Canonical(t, pg) != want {
		t.Fatal("commit failure changed destination")
	}
	if err := pg.Exec("DROP TRIGGER fail_stage3_commit ON users").Error; err != nil {
		t.Fatal(err)
	}
	// Terminate the actual source transaction after a read, not merely a mock error.
	err := pg.Transaction(func(tx *gorm.DB) error {
		var pid int
		if err := tx.Raw("SELECT pg_backend_pid()").Scan(&pid).Error; err != nil {
			return err
		}
		if err := pg.Exec("SELECT pg_terminate_backend(?)", pid).Error; err != nil {
			return err
		}
		return tx.Exec("UPDATE users SET password='must-not-save'").Error
	})
	if err == nil {
		t.Fatal("terminated connection reported success")
	}
	if stage3Canonical(t, pg) != want {
		t.Fatal("disconnect changed committed data")
	}
}
