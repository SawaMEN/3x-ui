package service

import (
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestAuditHwidIgnoresMissingAndDisabledOwners(t *testing.T) {
	stage6DB(t)
	svc := &ClientService{}
	rec := seedHwidClient(t, 0)
	if err := database.GetDB().Model(rec).UpdateColumn("enable", false).Error; err != nil {
		t.Fatal(err)
	}
	for _, subID := range []string{"missing-subscription", rec.SubID} {
		if _, err := svc.EnforceHwidForSubID(subID, HwidRequest{Hwid: "device-not-registered"}); err != nil {
			t.Fatal(err)
		}
	}
	var count int64
	if err := database.GetDB().Model(&model.ClientHwid{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("missing/disabled owner generated %d device rows", count)
	}
}

func TestAuditHwidUnlimitedTrackingFailureDoesNotDeny(t *testing.T) {
	db := stage6DB(t)
	rec := seedHwidClient(t, 0)
	statement := `CREATE TRIGGER reject_hwid_tracking BEFORE INSERT ON client_hwids BEGIN SELECT RAISE(ABORT,'synthetic tracking failure'); END`
	if db.Name() == "postgres" {
		statement = `CREATE FUNCTION reject_hwid_tracking() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'synthetic tracking failure'; END $$; CREATE TRIGGER reject_hwid_tracking BEFORE INSERT ON client_hwids FOR EACH ROW EXECUTE FUNCTION reject_hwid_tracking()`
	}
	if err := db.Exec(statement).Error; err != nil {
		t.Fatal(err)
	}
	result, err := (&ClientService{}).EnforceHwidForSubID(rec.SubID, HwidRequest{Hwid: "device-unlimited"})
	if err != nil || !result.Allowed || result.Active {
		t.Fatalf("best-effort tracking denied unlimited owner: %+v %v", result, err)
	}
}

func TestAuditHwidZeroToLimitedParentLock(t *testing.T) {
	if os.Getenv("XUI_DB_TYPE") != "postgres" {
		t.Skip("requires PostgreSQL row locks")
	}
	db := stage6DB(t)
	rec := seedHwidClient(t, 0)
	svc := &ClientService{}
	if r, err := svc.EnforceHwidForSubID(rec.SubID, HwidRequest{Hwid: "existing-device"}); err != nil || !r.Allowed {
		t.Fatal(r, err)
	}
	tx := db.Begin()
	defer tx.Rollback()
	var parent model.ClientRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&parent, rec.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Model(&parent).UpdateColumn("limit_hwid", 1).Error; err != nil {
		t.Fatal(err)
	}
	read := make(chan struct{})
	var once sync.Once
	const callback = "audit:initial-hwid-limit"
	if err := db.Callback().Row().After("gorm:row").Register(callback, func(q *gorm.DB) {
		if strings.Contains(q.Statement.SQL.String(), "MAX(limit_hwid)") {
			once.Do(func() { close(read) })
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Row().Remove(callback)
	type outcome struct {
		result HwidGateResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		r, err := svc.EnforceHwidForSubID(rec.SubID, HwidRequest{Hwid: "new-device"})
		done <- outcome{r, err}
	}()
	select {
	case <-read:
	case <-time.After(5 * time.Second):
		t.Fatal("initial cap query did not run")
	}
	var got outcome
	early := false
	select {
	case got = <-done:
		early = true
		t.Error("unlimited gate ignored locked owner while limit was enabled")
	case <-time.After(200 * time.Millisecond):
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	if !early {
		select {
		case got = <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("gate did not resume after commit")
		}
	}
	if got.err != nil || got.result.Allowed || got.result.Limit != 1 {
		t.Fatalf("newly enabled device cap bypassed: %+v %v", got.result, got.err)
	}
	var count int64
	if err := db.Model(&model.ClientHwid{}).Where("sub_id = ?", rec.SubID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("device rows = %d, want 1", count)
	}
}
