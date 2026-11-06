package database

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestStage3PostgresNativeBackup(t *testing.T) {
	pg, dsn := stage3Postgres(t)
	path := stage3PopulatedLegacy(t)
	if err := MigrateData(path, dsn); err != nil {
		t.Fatal(err)
	}
	want := stage3Canonical(t, pg)
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := cfg.RuntimeParams["search_path"]
	// Credentials stay in child environment, never in process arguments or logs.
	env := append(os.Environ(), "PGHOST="+cfg.Host, fmt.Sprintf("PGPORT=%d", cfg.Port), "PGUSER="+cfg.User, "PGPASSWORD="+cfg.Password, "PGDATABASE="+cfg.Database, "PGSSLMODE=disable")
	run := func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %w: %s", name, err, out)
		}
		return nil
	}
	dump := filepath.Join(t.TempDir(), "native.dump")
	if err := run("pg_dump", "--format=custom", "--schema="+schema, "--file="+dump); err != nil {
		t.Fatal(err)
	}
	if err := pg.Exec("ALTER SCHEMA " + schema + " RENAME TO " + schema + "_saved").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pg.Exec("DROP SCHEMA " + schema + "_saved CASCADE").Error })
	data, err := os.ReadFile(dump)
	if err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(t.TempDir(), "truncated.dump")
	if err := os.WriteFile(bad, data[:len(data)/2], 0600); err != nil {
		t.Fatal(err)
	}
	if err := run("pg_restore", "--dbname="+cfg.Database, "--single-transaction", "--exit-on-error", bad); err == nil {
		t.Fatal("truncated native archive accepted")
	}
	var exists bool
	if err := pg.Raw("SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=?)", schema).Scan(&exists).Error; err != nil || exists {
		t.Fatalf("partial native restore persisted: %v", err)
	}
	if err := run("pg_restore", "--dbname="+cfg.Database, "--single-transaction", "--exit-on-error", dump); err != nil {
		t.Fatal(err)
	}
	restored, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer closeGorm(restored)
	if stage3Canonical(t, restored) != want {
		t.Fatal("native backup/restore changed stored credentials/entities")
	}
}
