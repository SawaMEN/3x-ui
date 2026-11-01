package database

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// migrationModels is the FK-aware order in which tables are created and copied
// during `x-ui migrate-db --dsn` (SQLite → PostgreSQL data migration) and in
// related tests.
//
// Important: When adding a new top-level model (like OutboundSubscription),
// you must add it here **in addition to** allModels() in internal/database/db.go;
// TestMigrationModelsMatchPanelModels fails when the two lists drift apart.
// This list is used for:
//   - Creating the destination schema during cross-DB migration
//   - Truncating tables
//   - Copying data row-by-row
//   - Resyncing Postgres sequences after bulk insert
//
// DumpSQLite / RestoreSQLite are schema-introspective (they read sqlite_master)
// so they do not need manual updates.
func migrationModels() []any {
	return []any{
		&model.User{},
		&model.UserSession{},
		&model.Setting{},
		&model.HistoryOfSeeders{},
		&model.Node{},
		&model.ApiToken{},
		&model.RoutingPreset{},
		&model.RoutingList{},
		&model.Inbound{},
		&coremodel.ClientTraffic{},
		&model.OutboundTraffics{},
		&model.InboundClientIps{},
		&model.ClientRecord{},
		&model.ClientInbound{},
		&model.ClientHwid{},
		&model.ClientExternalLink{},
		&model.ClientGroup{},
		&model.InboundFallback{},
		&model.Host{},
		&model.HostTombstone{},
		&model.NodeClientTraffic{},
		&model.NodeClientIp{},
		&model.ClientGlobalTraffic{},
		&model.TelemtIPHistory{},
		&model.TelemtUserHistory{},
		&model.TelemtIPGeo{},
		&model.OutboundSubscription{},
		&model.SubBalancer{},
		&model.TuicTrafficReceipt{},
		&model.CoreTrafficCheckpoint{},
	}
}

// MigrateData copies every row from the configured SQLite file at srcPath into
// a fresh PostgreSQL database described by dstDSN. The destination tables are
// (re)created with AutoMigrate; truncate and copy then run in one transaction,
// so a failed migration leaves the destination data unchanged. Source data is
// left untouched.
func MigrateData(srcPath, dstDSN string) error {
	if _, err := os.Stat(srcPath); err != nil {
		return fmt.Errorf("source sqlite not found at %s: %w", srcPath, err)
	}
	if dstDSN == "" {
		return errors.New("destination DSN is required")
	}

	if err := os.MkdirAll(path.Dir(srcPath), 0o755); err != nil {
		return err
	}

	srcDSN := srcPath + "?_journal_mode=WAL&_busy_timeout=10000"
	src, err := gorm.Open(sqlite.Open(srcDSN), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return fmt.Errorf("open sqlite source: %w", err)
	}
	srcSQL, err := src.DB()
	if err != nil {
		return err
	}
	defer srcSQL.Close()

	dst, err := gorm.Open(postgres.Open(dstDSN), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return fmt.Errorf("open postgres destination: %w", err)
	}
	dstSQL, err := dst.DB()
	if err != nil {
		return err
	}
	defer dstSQL.Close()
	dstSQL.SetConnMaxLifetime(time.Hour)

	log.Println("Creating destination schema...")
	for _, m := range migrationModels() {
		if err := dst.AutoMigrate(m); err != nil {
			return fmt.Errorf("AutoMigrate %T: %w", m, err)
		}
	}

	totalRows := 0
	txErr := dst.Transaction(func(tx *gorm.DB) error {
		// AutoMigrate re-creates the legacy client_traffics -> inbounds foreign key,
		// but the running panel drops it (see dropLegacyForeignKeys) and tolerates
		// client_traffics rows whose inbound was deleted. Drop it here too so copying
		// such orphaned rows can't fail with an fk_inbounds_client_stats violation.
		if err := tx.Exec("ALTER TABLE client_traffics DROP CONSTRAINT IF EXISTS fk_inbounds_client_stats").Error; err != nil {
			return fmt.Errorf("drop legacy foreign key: %w", err)
		}

		// Empty the destination tables before copying: a fresh PostgreSQL DB
		// already holds an auto-seeded admin (id=1) from any prior panel start,
		// so a plain INSERT with explicit ids would collide on users_pkey. Only
		// the panel's own tables are cleared, and a failure anywhere in this
		// transaction rolls the clear back with everything else.
		if err := truncatePostgresTables(tx, migrationModels()); err != nil {
			return fmt.Errorf("clear destination tables: %w", err)
		}

		for _, m := range migrationModels() {
			n, err := copyTable(src, tx, m)
			if err != nil {
				return fmt.Errorf("copy %T: %w", m, err)
			}
			totalRows += n
			log.Printf("  %-32s %d rows", reflect.TypeOf(m).Elem().Name(), n)
		}
		return nil
	})
	if txErr != nil {
		return txErr
	}

	// setval is never rolled back by PostgreSQL, so sequences are resynced only
	// after the transaction has committed.
	if err := resetPostgresSequences(dst); err != nil {
		log.Printf("warning: failed to reset some postgres sequences: %v", err)
	}

	log.Printf("Migration complete: %d rows across %d tables.", totalRows, len(migrationModels()))
	log.Println("Set XUI_DB_TYPE=postgres and XUI_DB_DSN=... in /etc/default/x-ui, then restart x-ui.")
	return nil
}

// MigrateLegacyData imports an older SQLite panel into PostgreSQL without
// altering the user's source file. Unlike a regular import, older panels may
// lack entire tables or columns that today's migrationModels enumerates.
// Snapshot the live SQLite database (including its WAL) first, upgrade only
// that private snapshot, then use the strict transactional row copier.
func MigrateLegacyData(srcPath, dstDSN string) error {
	if dstDSN == "" {
		return errors.New("destination DSN is required")
	}
	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("source sqlite not found at %s: %w", srcPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("source sqlite %q is not a regular file", srcPath)
	}
	stage, err := os.MkdirTemp("", "3x-ui-legacy-migration-*")
	if err != nil {
		return fmt.Errorf("create migration snapshot directory: %w", err)
	}
	defer os.RemoveAll(stage)
	snapshot := filepath.Join(stage, "source.db")

	src, err := gorm.Open(sqlite.Open(srcPath+"?_busy_timeout=10000"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return fmt.Errorf("open legacy sqlite: %w", err)
	}
	sqlDB, err := src.DB()
	if err != nil {
		return err
	}
	// VACUUM INTO takes a transactionally consistent snapshot, unlike a raw
	// file copy that could omit uncheckpointed SQLite WAL transactions.
	backupErr := src.Exec("VACUUM INTO ?", snapshot).Error
	closeErr := sqlDB.Close()
	if backupErr != nil {
		return fmt.Errorf("snapshot legacy sqlite: %w", backupErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close legacy sqlite: %w", closeErr)
	}
	if err := PrepareSQLiteForMigration(snapshot); err != nil {
		return fmt.Errorf("prepare legacy sqlite snapshot: %w", err)
	}
	return MigrateData(snapshot, dstDSN)
}

// ExportPostgresToSQLite copies every row from PostgreSQL into a portable
// SQLite file. Publish the finished backup atomically: failure to reach the
// source or copy a table must never destroy a previous valid export.
func ExportPostgresToSQLite(srcDSN, dstPath string) error {
	if srcDSN == "" {
		return errors.New("source DSN is required")
	}
	if err := os.MkdirAll(path.Dir(dstPath), 0o755); err != nil {
		return err
	}

	src, err := gorm.Open(postgres.Open(srcDSN), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return fmt.Errorf("open postgres source: %w", err)
	}
	srcSQL, err := src.DB()
	if err != nil {
		return err
	}
	defer srcSQL.Close()

	// Keep the temporary file beside the requested destination so rename is
	// atomic on the same filesystem. CreateTemp also protects its permissions.
	tmp, err := os.CreateTemp(path.Dir(dstPath), ".x-ui-export-*.db")
	if err != nil {
		return fmt.Errorf("create sqlite export: %w", err)
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temporary sqlite export: %w", err)
	}
	defer os.Remove(tmpPath)

	// No WAL: a closed and renamed main database file is self-contained.
	dst, err := gorm.Open(sqlite.Open(tmpPath+"?_busy_timeout=10000"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return fmt.Errorf("open sqlite destination: %w", err)
	}
	dstSQL, err := dst.DB()
	if err != nil {
		return err
	}
	defer dstSQL.Close()

	if err := copyAllModels(src, dst); err != nil {
		return err
	}
	if err := dstSQL.Close(); err != nil {
		return fmt.Errorf("close sqlite export: %w", err)
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		return fmt.Errorf("publish sqlite export: %w", err)
	}
	return nil
}

// copyAllModels (re)creates the schema on dst and copies every migrated table
// from src to dst in FK-safe order. src/dst may be any gorm backend.
func copyAllModels(src, dst *gorm.DB) error {
	for _, m := range migrationModels() {
		if err := dst.AutoMigrate(m); err != nil {
			return fmt.Errorf("AutoMigrate %T: %w", m, err)
		}
	}
	for _, m := range migrationModels() {
		if _, err := copyTable(src, dst, m); err != nil {
			return fmt.Errorf("copy %T: %w", m, err)
		}
	}
	return nil
}

func copyTable(src, dst *gorm.DB, mdl any) (int, error) {
	const batchSize = 500

	sliceType := reflect.SliceOf(reflect.PointerTo(reflect.TypeOf(mdl).Elem()))

	stmt := &gorm.Statement{DB: src}
	if err := stmt.Parse(mdl); err != nil {
		return 0, err
	}
	order := strings.Join(stmt.Schema.PrimaryFieldDBNames, ", ")
	table := stmt.Schema.Table
	columns := stmt.Schema.DBNames

	ctx := context.Background()
	total := 0
	for offset := 0; ; offset += batchSize {
		batchPtr := reflect.New(sliceType)
		q := src.Model(mdl).Limit(batchSize).Offset(offset)
		if order != "" {
			q = q.Order(order)
		}
		if err := q.Find(batchPtr.Interface()).Error; err != nil {
			return total, err
		}
		slice := batchPtr.Elem()
		n := slice.Len()
		if n == 0 {
			break
		}

		rows := make([]map[string]any, n)
		for i := range n {
			rv := reflect.Indirect(slice.Index(i))
			row := make(map[string]any, len(columns))
			for _, name := range columns {
				value, _ := stmt.Schema.FieldsByDBName[name].ValueOf(ctx, rv)
				row[name] = value
			}
			rows[i] = row
		}

		if err := dst.Table(table).CreateInBatches(rows, 200).Error; err != nil {
			return total, err
		}
		total += n
		if n < batchSize {
			break
		}
	}
	return total, nil
}

// truncatePostgresTables empties every migrated table on dst in a single
// statement, resetting identity sequences. CASCADE covers the inbound/client
// foreign keys regardless of insertion order. Only the panel's own tables are
// touched, never the rest of the schema.
func truncatePostgresTables(dst *gorm.DB, models []any) error {
	tables := make([]string, 0, len(models))
	for _, m := range models {
		stmt := &gorm.Statement{DB: dst}
		if err := stmt.Parse(m); err != nil {
			return err
		}
		tables = append(tables, `"`+stmt.Schema.Table+`"`)
	}
	if len(tables) == 0 {
		return nil
	}
	log.Println("Clearing destination tables...")
	return dst.Exec("TRUNCATE TABLE " + strings.Join(tables, ", ") + " RESTART IDENTITY CASCADE").Error
}

// resetPostgresSequences advances each migrated table's id sequence past MAX(id),
// otherwise the next INSERT-without-id would clash with copied rows.
func resetPostgresSequences(dst *gorm.DB) error {
	return resyncPostgresSequences(dst, migrationModels())
}

// resyncPostgresSequences sets each model's id sequence to MAX(id); idempotent. Id-less
// composite-PK tables are skipped — Postgres rejects MAX(id) at parse time and logs it (#5665).
func resyncPostgresSequences(db *gorm.DB, models []any) error {
	for _, m := range models {
		t, ok := tableWithIdColumn(db, m)
		if !ok {
			continue
		}
		// t comes from the trusted model set parsed by GORM, not user input, so
		// interpolating it as an identifier is safe. We ignore errors per-table.
		_ = db.Exec(
			`SELECT setval(pg_get_serial_sequence(?, 'id'), COALESCE((SELECT MAX(id) FROM "`+t+`"), 1), true)
			 WHERE pg_get_serial_sequence(?, 'id') IS NOT NULL`,
			t, t,
		).Error
	}
	return nil
}

// tableWithIdColumn resolves a model's table name and reports whether its GORM
// schema maps an "id" database column.
func tableWithIdColumn(db *gorm.DB, m any) (string, bool) {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(m); err != nil {
		return "", false
	}
	if stmt.Schema == nil || stmt.Schema.LookUpField("id") == nil {
		return "", false
	}
	return stmt.Table, true
}

// PrepareSQLiteForMigration rejects SQLite files that are not a panel database
// before the caller causes any downtime, then AutoMigrates the panel schema
// onto the file so backups from older versions gain the newer tables and
// columns the row copy reads. Data-level upgrades are not needed here: they
// run dialect-agnostically on the destination via InitDB after the import.
func PrepareSQLiteForMigration(dbPath string) error {
	gdb, err := gorm.Open(sqlite.Open(dbPath+"?_busy_timeout=10000"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return err
	}
	sqlDB, err := gdb.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	for _, table := range []string{"users", "settings", "inbounds"} {
		if !sqliteTableExists(sqlDB, table) {
			return fmt.Errorf("not a 3x-ui panel database: required table %q is missing", table)
		}
	}
	for _, m := range migrationModels() {
		if err := gdb.AutoMigrate(m); err != nil && !isIgnorableDuplicateColumnErr(gdb, err, m) {
			return fmt.Errorf("upgrade panel schema for %T: %w", m, err)
		}
	}
	return nil
}
