package database

import (
	"database/sql"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// SQLite's go-sqlite3 driver uses the DSN's transaction mode even for
// sql.TxOptions.ReadOnly. The main pool intentionally uses BEGIN IMMEDIATE
// for write transactions. Keep consistent snapshots on separate, query-only
// connections using BEGIN DEFERRED instead of reserving the writer lock.
type sqliteSnapshotPools struct {
	owner, reader *sql.DB
}

var sqliteSnapshots atomic.Pointer[sqliteSnapshotPools]

func openSQLiteSnapshotPool(path string, owner *sql.DB) error {
	reader, err := sql.Open("sqlite3", path+"?_busy_timeout=10000&_txlock=deferred&_query_only=1")
	if err != nil {
		return err
	}
	reader.SetMaxOpenConns(2)
	reader.SetMaxIdleConns(1)
	reader.SetConnMaxLifetime(time.Hour)
	reader.SetConnMaxIdleTime(5 * time.Minute)
	if err := reader.Ping(); err != nil {
		_ = reader.Close()
		return err
	}
	sqliteSnapshots.Store(&sqliteSnapshotPools{owner: owner, reader: reader})
	return nil
}

func closeSQLiteSnapshotPool() error {
	if pools := sqliteSnapshots.Swap(nil); pools != nil {
		return pools.reader.Close()
	}
	return nil
}

// ReadSnapshot runs read-only queries in a consistent transaction. Call only
// with a root database/session, never from an existing write transaction.
// Share the GORM callbacks, naming strategy and context with the main handle.
func ReadSnapshot(base *gorm.DB, fn func(*gorm.DB) error) error {
	opts := &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead}
	if pools := sqliteSnapshots.Load(); pools != nil && base.Name() == DialectSQLite && base.Statement.ConnPool == pools.owner {
		session := base.Session(&gorm.Session{NewDB: true, Context: base.Statement.Context})
		session.Config.ConnPool = pools.reader
		session.Statement.ConnPool = pools.reader
		return session.Transaction(fn, opts)
	}
	// PostgreSQL and standalone/test databases use their normal transaction API.
	return base.Transaction(fn, opts)
}
