package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"os/exec"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

// CoreMigrationPostgresSnapshot runs before InitDB, including its schema changes.
// The DSN and password stay out of command arguments and the migration manifest.
func CoreMigrationPostgresSnapshot(ctx context.Context, dst string) error {
	env, dbname, err := pgConnEnv(config.GetDBDSN())
	if err != nil {
		return err
	}
	restore, err := exec.LookPath("pg_restore")
	if err != nil {
		return err
	}
	dump, err := exec.LookPath("pg_dump")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, dump, "--format=custom", "--no-owner", "--no-privileges", "--dbname", dbname)
	cmd.Env, cmd.Stdout = env, file
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	syncErr := file.Sync()
	closeErr := file.Close()
	if runErr != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("pg_dump failed: %w", runErr)
	}
	if syncErr != nil {
		return syncErr
	}
	if closeErr != nil {
		return closeErr
	}
	return coreMigrationPostgresCanRead(ctx, restore, dst)
}

func CoreMigrationPostgresRestore(ctx context.Context, src string) error {
	env, dbname, err := pgConnEnv(config.GetDBDSN())
	if err != nil {
		return err
	}
	bin, err := exec.LookPath("pg_restore")
	if err != nil {
		return err
	}
	if err := coreMigrationPostgresCanRead(ctx, bin, src); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin, "--clean", "--if-exists", "--single-transaction", "--exit-on-error", "--no-owner", "--no-privileges", "--dbname", dbname, src)
	cmd.Env = env
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("restore PostgreSQL migration snapshot: %w", err)
	}
	return nil
}

// CoreMigrationPostgresTables records public tables before InitDB can add new ones.
func CoreMigrationPostgresTables(ctx context.Context) ([]string, error) {
	db, err := gorm.Open(postgres.Open(config.GetDBDSN()), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	defer sqlDB.Close()
	var names []string
	err = db.WithContext(ctx).Raw("SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename").Scan(&names).Error
	return names, err
}

func CoreMigrationPostgresRemoveAddedTables(ctx context.Context, original []string) error {
	db, err := gorm.Open(postgres.Open(config.GetDBDSN()), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	var current []string
	if err := db.WithContext(ctx).Raw("SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename").Scan(&current).Error; err != nil {
		return err
	}
	known := map[string]bool{}
	for _, name := range original {
		known[name] = true
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, name := range current {
			if !known[name] {
				if err := tx.Exec(`DROP TABLE public."` + strings.ReplaceAll(name, `"`, `""`) + `" CASCADE`).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// CoreMigrationPostgresFingerprint binds a prepared snapshot to its current data,
// schema and sequence positions. Rows stream through a repeatable-read snapshot;
// the database sorts canonical JSON rather than holding the dump in panel RAM.
func CoreMigrationPostgresFingerprint(ctx context.Context) (string, error) {
	db, err := gorm.Open(postgres.Open(config.GetDBDSN()), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return "", err
	}
	connection, err := db.DB()
	if err != nil {
		return "", err
	}
	defer connection.Close()
	digest := sha256.New()
	err = db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tables []string
		if err := tx.Raw(`SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename COLLATE "C"`).Scan(&tables).Error; err != nil {
			return err
		}
		for _, name := range tables {
			hashMigrationValue(digest, "table:"+name)
			rows, err := tx.Raw(`SELECT value_json FROM (SELECT to_jsonb(t)::text AS value_json FROM public."` + strings.ReplaceAll(name, `"`, `""`) + `" t) source ORDER BY value_json COLLATE "C"`).Rows()
			if err != nil {
				return err
			}
			if err := hashMigrationRows(digest, rows); err != nil {
				return err
			}
		}
		rows, err := tx.Raw(`SELECT to_jsonb(c)::text FROM (SELECT table_name, column_name, ordinal_position, data_type, udt_name, is_nullable, column_default FROM information_schema.columns WHERE table_schema = 'public' ORDER BY table_name COLLATE "C", ordinal_position) c`).Rows()
		if err != nil {
			return err
		}
		hashMigrationValue(digest, "schema")
		if err := hashMigrationRows(digest, rows); err != nil {
			return err
		}
		rows, err = tx.Raw(`SELECT to_jsonb(s)::text FROM (SELECT sequencename, start_value, min_value, max_value, increment_by, cycle, cache_size, last_value FROM pg_sequences WHERE schemaname = 'public' ORDER BY sequencename COLLATE "C") s`).Rows()
		if err != nil {
			return err
		}
		hashMigrationValue(digest, "sequences")
		return hashMigrationRows(digest, rows)
	}, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func hashMigrationValue(digest hash.Hash, value string) {
	var size [8]byte
	binary.BigEndian.PutUint64(size[:], uint64(len(value)))
	digest.Write(size[:])
	digest.Write([]byte(value))
}

func hashMigrationRows(digest hash.Hash, rows *sql.Rows) error {
	defer rows.Close()
	for rows.Next() {
		var row string
		if err := rows.Scan(&row); err != nil {
			return err
		}
		hashMigrationValue(digest, row)
	}
	return rows.Err()
}

// Unlike interactive imports, offline recovery must obey its cancellation
// deadline even when the restore utility stalls during its archive preflight.
func coreMigrationPostgresCanRead(ctx context.Context, bin, path string) error {
	cmd := exec.CommandContext(ctx, bin, "--list", path)
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("pg_restore cannot read migration snapshot: %w", err)
	}
	return nil
}
