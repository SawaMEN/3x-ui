package job

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestShutdownTrafficJournalsSurviveDBFailureWithoutMaintenance(t *testing.T) {
	for _, source := range []struct {
		name     string
		protocol model.Protocol
	}{{"mieru", model.Mieru}, {"mtproto", model.MTProto}, {"externalvpn", model.TrustTunnel}} {
		t.Run(source.name, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XUI_DB_FOLDER", dir)
			dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
			db := database.GetDB()
			ib := model.Inbound{Tag: "tail-direct", Protocol: source.protocol, Settings: `{"clients":[]}`}
			routed := model.Inbound{Tag: "tail-routed", Protocol: source.protocol, Settings: `{"clients":[]}`}
			expired := model.Inbound{Tag: "must-not-reconcile", Protocol: model.VLESS, Settings: `{"clients":[]}`, Enable: true, ExpiryTime: 1}
			for _, i := range []*model.Inbound{&ib, &routed, &expired} {
				if err := db.Create(i).Error; err != nil {
					t.Fatal(err)
				}
			}
			row := coremodel.ClientTraffic{Email: "shutdown@example.test", InboundId: ib.Id, Enable: true}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			owners, err := service.CaptureSidecarTrafficOwners([]model.Protocol{source.protocol})
			if err != nil {
				t.Fatal(err)
			}
			fail := true
			cb := "test:final-traffic-write-failure"
			if err := db.Callback().Update().Before("gorm:update").Register(cb, func(tx *gorm.DB) {
				if fail && tx.Statement.Table == "inbounds" {
					tx.AddError(errors.New("synthetic database failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer db.Callback().Update().Remove(cb)
			// Retain a previous periodic sample and then fail DB writes during the
			// final poll as well. Both batches must survive, even though replay fails.
			var journal sidecarTrafficJournal
			if _, _, err := journal.commit(source.name, owners, &service.InboundService{}, []*coremodel.Traffic{{IsInbound: true, Tag: ib.Tag, Up: 7}}, []*coremodel.ClientTraffic{{Email: row.Email, Up: 7}}); err == nil {
				t.Fatal("expected periodic DB failure")
			}
			var mu sync.Mutex
			calls, stops := 0, 0
			src := shutdownTrafficSource{name: source.name, protocols: []model.Protocol{source.protocol}, mu: &mu, collect: func() ([]shutdownTrafficSample, error) {
				calls++
				return []shutdownTrafficSample{{tag: ib.Tag, email: row.Email, up: 11, down: 13}, {tag: routed.Tag, email: row.Email, up: 17, down: 19, routed: true}}, nil
			}, stop: func() { stops++ }}
			if err := flushShutdownTrafficSource(src); err == nil {
				t.Fatal("expected shutdown DB failure")
			}
			if calls != 1 || stops != 1 {
				t.Fatalf("collect/stop calls: %d/%d", calls, stops)
			}
			paths, err := filepath.Glob(filepath.Join(sidecarJournalDir(source.name), "*.json"))
			if err != nil || len(paths) != 2 {
				t.Fatalf("lost periodic or final WAL: %v %v", paths, err)
			}
			fail = false
			apply := func(batch service.SidecarTrafficBatch) (bool, bool, error) {
				return false, false, (&service.InboundService{}).PersistSidecarTrafficBatch(batch)
			}
			if _, _, err := journal.replayWith(source.name, apply); err != nil {
				t.Fatal(err)
			}
			if _, _, err := journal.replayWith(source.name, apply); err != nil {
				t.Fatal(err)
			}
			if err := db.First(&row, row.Id).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&ib, ib.Id).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&routed, routed.Id).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&expired, expired.Id).Error; err != nil {
				t.Fatal(err)
			}
			if row.Up != 35 || row.Down != 32 || ib.Up != 18 || ib.Down != 13 || routed.Up != 0 || routed.Down != 0 {
				t.Fatalf("wrong shutdown totals client=%d/%d direct=%d/%d routed=%d/%d", row.Up, row.Down, ib.Up, ib.Down, routed.Up, routed.Down)
			}
			if !expired.Enable {
				t.Fatal("shutdown ran quota/runtime lifecycle maintenance")
			}
		})
	}
}

func TestShutdownTrafficDoesNotConsumeWithoutOwnersAndStillStops(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	db := database.GetDB()
	cb := "test:no-shutdown-owners"
	if err := db.Callback().Query().Before("gorm:query").Register(cb, func(tx *gorm.DB) {
		if tx.Statement.Table == "inbounds" {
			tx.AddError(errors.New("synthetic owner query failure"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove(cb)
	var mu sync.Mutex
	stopped := false
	err := flushShutdownTrafficSource(shutdownTrafficSource{name: "mieru", protocols: []model.Protocol{model.Mieru}, mu: &mu, collect: func() ([]shutdownTrafficSample, error) {
		t.Fatal("advanced cursor without stable owner IDs")
		return nil, nil
	}, stop: func() { stopped = true }})
	if err == nil || !stopped {
		t.Fatalf("error=%v stopped=%v", err, stopped)
	}
}

func TestShutdownTrafficPersistsPartialSampleAndReportsCollectorError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	db := database.GetDB()
	ib := model.Inbound{Tag: "raw-mtproto", Protocol: model.MTProto, Settings: `{"clients":[]}`}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	row := coremodel.ClientTraffic{InboundId: ib.Id, Email: "partial@example.test"}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	partialErr := errors.New("synthetic WEB metrics outage")
	var mu sync.Mutex
	stopped := false
	err := flushShutdownTrafficSource(shutdownTrafficSource{name: "mtproto", mu: &mu, protocols: []model.Protocol{model.MTProto}, collect: func() ([]shutdownTrafficSample, error) {
		return []shutdownTrafficSample{{tag: ib.Tag, email: row.Email, down: 83}}, partialErr
	}, stop: func() { stopped = true }})
	if !errors.Is(err, partialErr) || !stopped {
		t.Fatalf("partial error=%v stop=%v", err, stopped)
	}
	if err := db.First(&row, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if row.Down != 83 {
		t.Fatalf("lost successful raw sample after WEB failure: %d", row.Down)
	}
}
