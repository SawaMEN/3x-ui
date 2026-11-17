package database

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestClampedAddStartsMissingCounterAtZero(t *testing.T) {
	conn, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "counters.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err = conn.Exec(`CREATE TABLE counters(value INTEGER); INSERT INTO counters VALUES(NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(`UPDATE counters SET value=`+clampedAddExpr("value", false), int64(7)); err != nil {
		t.Fatal(err)
	}
	var got sql.NullInt64
	if err = conn.QueryRow(`SELECT value FROM counters`).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if !got.Valid || got.Int64 != 7 {
		t.Fatalf("traffic delta discarded for NULL counter: %+v", got)
	}
}

func TestTrafficRepairNormalizesMissingCounters(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "panel.db")
	if err := InitDB(dbPath); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = CloseDB() })
	if err := db.Exec(`INSERT INTO client_traffics(email,up,down) VALUES('legacy-null',NULL,NULL)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := repairOverflowedTrafficCounters(); err != nil {
		t.Fatal(err)
	}
	var row struct{ Up, Down sql.NullInt64 }
	if err := db.Raw(`SELECT up,down FROM client_traffics WHERE email='legacy-null'`).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if !row.Up.Valid || !row.Down.Valid || row.Up.Int64 != 0 || row.Down.Int64 != 0 {
		t.Fatalf("legacy counters remain missing: %+v", row)
	}
}
