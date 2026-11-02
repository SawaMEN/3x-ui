package service

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/testpg"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func stage6DB(t *testing.T) *gorm.DB {
	t.Helper()
	cleanup, err := testpg.IsolatePackage("stage6")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cleanup)
	return initTrafficTestDB(t)
}

func TestStage6HwidDeviceCap(t *testing.T) {
	db := stage6DB(t)
	rec := seedHwidClient(t, 5)
	svc := &ClientService{}
	for i := range 4 {
		if r, e := svc.EnforceHwidForSubID(rec.SubID, HwidRequest{Hwid: fmt.Sprintf("existing-device-%d", i)}); e != nil || !r.Allowed {
			t.Fatal(r, e)
		}
	}
	var accepted atomic.Int32
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range 24 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			r, e := svc.EnforceHwidForSubID(rec.SubID, HwidRequest{Hwid: fmt.Sprintf("new-device-%d", i)})
			if e != nil {
				t.Error(e)
			} else if r.Allowed {
				accepted.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	var count int64
	if err := db.Model(&model.ClientHwid{}).Where("sub_id = ?", rec.SubID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if accepted.Load() != 1 || count != 5 {
		t.Fatalf("last slot admitted %d devices, stored=%d", accepted.Load(), count)
	}
	r, e := svc.EnforceHwidForSubID(rec.SubID, HwidRequest{Hwid: "existing-device-0"})
	if e != nil || !r.Allowed || r.Registered != 5 {
		t.Fatal("existing device lost its slot", r, e)
	}
}

// Independent transactions model another panel process: an in-process mutex
// alone cannot pass this test. The gate must wait and re-read the committed cap.
func TestStage6HwidGateParentLock(t *testing.T) {
	if os.Getenv("XUI_DB_TYPE") != "postgres" {
		t.Skip("requires PostgreSQL row locks")
	}
	db := stage6DB(t)
	rec := seedHwidClient(t, 5)
	svc := &ClientService{}
	if r, e := svc.EnforceHwidForSubID(rec.SubID, HwidRequest{Hwid: "existing-device"}); e != nil || !r.Allowed {
		t.Fatal(r, e)
	}
	tx := db.Begin()
	defer tx.Rollback()
	var parent model.ClientRecord
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&parent, rec.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Model(&parent).Update("limit_hwid", 1).Error; err != nil {
		t.Fatal(err)
	}
	read := make(chan struct{})
	var once sync.Once
	const callback = "stage6:observed-hwid-limit"
	if err := db.Callback().Row().After("gorm:row").Register(callback, func(q *gorm.DB) {
		if strings.Contains(q.Statement.SQL.String(), "MAX(limit_hwid)") {
			once.Do(func() { close(read) })
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Row().Remove(callback)
	type result struct {
		gate HwidGateResult
		err  error
	}
	done := make(chan result, 1)
	go func() {
		r, e := svc.EnforceHwidForSubID(rec.SubID, HwidRequest{Hwid: "second-device"})
		done <- result{r, e}
	}()
	select {
	case <-read:
	case <-time.After(5 * time.Second):
		t.Fatal("gate never read its initial limit")
	}
	var got result
	early := false
	select {
	case got = <-done:
		early = true
		t.Error("HWID gate ignored a locked subscription parent")
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
	if got.err != nil || got.gate.Allowed || got.gate.Limit != 1 {
		t.Fatalf("committed cap ignored: %+v %v", got.gate, got.err)
	}
}

func TestStage6QuotaSQL(t *testing.T) {
	db := stage6DB(t)
	rows := []coremodel.ClientTraffic{
		{Email: "local@stage6", Enable: true, Total: database.TrafficMax, Up: database.TrafficMax - 1, Down: database.TrafficMax - 1},
		{Email: "global@stage6", Enable: true, Total: database.TrafficMax},
		{Email: "small@stage6", Enable: true, Total: 100, Up: 10, Down: 20},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := db.Create(&model.ClientGlobalTraffic{Email: rows[1].Email, Up: database.TrafficMax - 1, Down: database.TrafficMax - 1, UpdatedAt: now.UnixMilli()}).Error; err != nil {
		t.Fatal(err)
	}
	var spent []coremodel.ClientTraffic
	if err := db.Where(depletedClientsCond, now.UnixMilli(), globalTrafficFreshSince(now)).Find(&spent).Error; err != nil {
		t.Fatal("quota predicate overflowed", err)
	}
	if len(spent) != 2 {
		t.Fatalf("expected local and global depletion, got %+v", spent)
	}
}
