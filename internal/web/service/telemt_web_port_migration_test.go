package service

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
)

func seedTelemtPortMigration(t *testing.T, deadline time.Time) {
	t.Helper()
	setupConflictDB(t)
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		if err := saveSettingIn(tx, "webPort", "18080"); err != nil {
			return err
		}
		if err := saveSettingIn(tx, "subPort", "18081"); err != nil {
			return err
		}
		return saveTelemtPortMigration(tx, &TelemtPortMigration{
			ID: "test", OldPanel: 443, NewPanel: 18080,
			OldSubscription: 2096, NewSubscription: 18081, Deadline: deadline,
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func assertTelemtMigrationPorts(t *testing.T, panel, sub int) {
	t.Helper()
	settings := &SettingService{}
	if actual, err := settings.GetPort(); err != nil || actual != panel {
		t.Fatalf("panel port = %d, error = %v; want %d", actual, err, panel)
	}
	if actual, err := settings.GetSubPort(); err != nil || actual != sub {
		t.Fatalf("subscription port = %d, error = %v; want %d", actual, err, sub)
	}
}

func TestTelemtPortMigrationRequiresReadyAndMatchingID(t *testing.T) {
	seedTelemtPortMigration(t, time.Now().Add(time.Minute))
	if err := ConfirmTelemtPortMigration("test"); err == nil {
		t.Fatal("accepted confirmation before listener startup")
	}
	if err := MarkTelemtPortMigrationReady(); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmTelemtPortMigration("stale"); err == nil {
		t.Fatal("accepted stale migration identifier")
	}
	if err := ConfirmTelemtPortMigration("test"); err != nil {
		t.Fatal(err)
	}
	assertTelemtMigrationPorts(t, 18080, 18081)
	if m, err := TelemtPendingPortMigration(); err != nil || m != nil {
		t.Fatalf("journal remained after confirmation: %+v, %v", m, err)
	}
}

func TestTelemtPortMigrationRollbackOnTimeout(t *testing.T) {
	seedTelemtPortMigration(t, time.Now().Add(-time.Second))
	if err := MarkTelemtPortMigrationReady(); err == nil {
		t.Fatal("expired migration became ready")
	}
	if restored, err := RollbackTelemtPortMigration(true); err != nil || !restored {
		t.Fatalf("timeout recovery: %v %v", restored, err)
	}
	assertTelemtMigrationPorts(t, 443, 2096)
	if restored, err := RollbackTelemtPortMigration(true); err != nil || restored {
		t.Fatalf("rollback must be idempotent: %v %v", restored, err)
	}
}

func TestTelemtPortMigrationCrashRecoveryPreservesOtherEdits(t *testing.T) {
	seedTelemtPortMigration(t, time.Now().Add(time.Minute))
	if restored, err := RollbackTelemtPortMigration(true); err != nil || restored {
		t.Fatalf("premature rollback: %v %v", restored, err)
	}
	if err := (&SettingService{}).SetPort(19000); err != nil {
		t.Fatal(err)
	}
	if restored, err := RollbackTelemtPortMigration(false); err != nil || !restored {
		t.Fatalf("crash recovery: %v %v", restored, err)
	}
	assertTelemtMigrationPorts(t, 19000, 2096)
}
