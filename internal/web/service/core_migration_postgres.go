package service

import (
	"bytes"
	"context"
	"fmt"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"os"
	"os/exec"
	"strings"

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
	file, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
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
	return checkPgRestoreCanRead(restore, dst)
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
	if err := checkPgRestoreCanRead(bin, src); err != nil {
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
func CoreMigrationPostgresTables() ([]string, error) {
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
	err = db.Raw("SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename").Scan(&names).Error
	return names, err
}

func CoreMigrationPostgresRemoveAddedTables(original []string) error {
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
	if err := db.Raw("SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY tablename").Scan(&current).Error; err != nil {
		return err
	}
	known := map[string]bool{}
	for _, name := range original {
		known[name] = true
	}
	return db.Transaction(func(tx *gorm.DB) error {
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
