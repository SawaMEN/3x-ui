package database

import (
	"errors"
	"testing"

	"gorm.io/gorm"
)

type auditSequenceRow struct {
	ID int64 `gorm:"primaryKey;autoIncrement"`
}

func TestPostgresResyncPreservesSequenceProgress(t *testing.T) {
	for _, tc := range []struct {
		name  string
		setup string
		want  int64
	}{
		{"empty", "", 1},
		{"deleted-maximum", `INSERT INTO audit_sequence_rows DEFAULT VALUES; INSERT INTO audit_sequence_rows DEFAULT VALUES; INSERT INTO audit_sequence_rows DEFAULT VALUES; DELETE FROM audit_sequence_rows WHERE id = 3`, 4},
		{"explicit-import", `INSERT INTO audit_sequence_rows (id) VALUES (50)`, 51},
		{"uncalled-equal", `INSERT INTO audit_sequence_rows (id) VALUES (1)`, 2},
		{"empty-after-delete", `INSERT INTO audit_sequence_rows DEFAULT VALUES; INSERT INTO audit_sequence_rows DEFAULT VALUES; DELETE FROM audit_sequence_rows`, 3},
		{"reserved-sequence", `SELECT setval(pg_get_serial_sequence('audit_sequence_rows','id'), 100, false)`, 100},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g, _ := stage3Postgres(t)
			if err := g.AutoMigrate(&auditSequenceRow{}); err != nil {
				t.Fatal(err)
			}
			if tc.setup != "" {
				if err := g.Exec(tc.setup).Error; err != nil {
					t.Fatal(err)
				}
			}
			for range 2 {
				if err := resyncPostgresSequences(g, []any{&auditSequenceRow{}}); err != nil {
					t.Fatal(err)
				}
			}
			var row auditSequenceRow
			if err := g.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			if row.ID != tc.want {
				t.Fatalf("next ID = %d, want %d", row.ID, tc.want)
			}
		})
	}
}

func TestPostgresResyncDoesNotChangeSequenceOnRollback(t *testing.T) {
	g, _ := stage3Postgres(t)
	if err := g.AutoMigrate(&auditSequenceRow{}); err != nil {
		t.Fatal(err)
	}
	if err := g.Exec(`INSERT INTO audit_sequence_rows (id) VALUES (50)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := g.Transaction(func(tx *gorm.DB) error {
		if err := resyncPostgresSequences(tx, []any{&auditSequenceRow{}}); err != nil {
			return err
		}
		return gorm.ErrInvalidTransaction
	}); !errors.Is(err, gorm.ErrInvalidTransaction) {
		t.Fatalf("rollback: %v", err)
	}
	var row auditSequenceRow
	if err := g.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.ID != 1 {
		t.Fatalf("rolled-back repair changed sequence: next = %d", row.ID)
	}
}
