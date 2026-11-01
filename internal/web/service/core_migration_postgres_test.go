//go:build linux

package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCoreMigrationPostgresCommandsKeepCredentialsOutOfArguments(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "commands")
	t.Setenv("XUI_DB_DSN", "postgres://panel:migration-secret@127.0.0.1:5432/panel?sslmode=disable")
	t.Setenv("MIGRATION_COMMAND_LOG", log)
	script := `#!/bin/sh
if [ "$1" != --list ]; then [ "$PGPASSWORD" = migration-secret ] || exit 3; fi
printf '%s %s\n' "${0##*/}" "$*" >> "$MIGRATION_COMMAND_LOG"
case "${0##*/}" in
pg_dump) printf 'PGDMP test snapshot' ;;
pg_restore) exit 0 ;;
esac
`
	for _, name := range []string{"pg_dump", "pg_restore"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	dump := filepath.Join(dir, "database.dump")
	if err := CoreMigrationPostgresSnapshot(context.Background(), dump); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dump)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatal("dump permissions are not private")
	}
	if err := CoreMigrationPostgresRestore(context.Background(), dump); err != nil {
		t.Fatal(err)
	}
	commands, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	actual := string(commands)
	if strings.Contains(actual, "migration-secret") || strings.Contains(actual, "postgres://") {
		t.Fatal("connection secrets reached command arguments")
	}
	for _, flag := range []string{"--format=custom", "--list", "--clean", "--if-exists", "--single-transaction", "--exit-on-error", "--dbname panel"} {
		if !strings.Contains(actual, flag) {
			t.Fatalf("missing restore safety flag %s", flag)
		}
	}
}

func TestCoreMigrationPostgresSnapshotRequiresRestoreToolBeforeDump(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("XUI_DB_DSN", "postgres://panel:secret@localhost/panel")
	dump := filepath.Join(dir, "database.dump")
	if err := CoreMigrationPostgresSnapshot(context.Background(), dump); err == nil {
		t.Fatal("snapshot accepted without a restore tool")
	}
	if _, err := os.Stat(dump); !os.IsNotExist(err) {
		t.Fatal("non-restorable dump was created")
	}
}

func TestCoreMigrationPostgresArchiveProbeHonorsCancellation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_DSN", "postgres://panel:secret@localhost/panel")
	script := "#!/bin/sh\nexec /bin/sleep 30\n"
	if err := os.WriteFile(filepath.Join(dir, "pg_restore"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := CoreMigrationPostgresRestore(ctx, filepath.Join(dir, "database.dump"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("archive probe ignored cancellation", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("archive probe exceeded cancellation budget")
	}
}
