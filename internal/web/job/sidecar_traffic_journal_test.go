package job

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

func TestSidecarJournalSurvivesReplacementAndAmbiguousReplay(t *testing.T) {
	for _, source := range []string{"mieru", "mtproto", "externalvpn"} {
		t.Run(source, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XUI_DB_FOLDER", dir)
			dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
			db := database.GetDB()
			protocol := model.Mieru
			if source == "mtproto" {
				protocol = model.MTProto
			}
			if source == "externalvpn" {
				protocol = model.OpenFlux
			}
			ib := model.Inbound{Tag: "old-tag", Protocol: protocol, Settings: `{"clients":[]}`}
			if err := db.Create(&ib).Error; err != nil {
				t.Fatal(err)
			}
			client := coremodel.ClientTraffic{Email: "old@example.test", InboundId: ib.Id, Enable: true}
			if err := db.Create(&client).Error; err != nil {
				t.Fatal(err)
			}
			owners, err := service.CaptureSidecarTrafficOwners([]model.Protocol{protocol})
			if err != nil {
				t.Fatal(err)
			}
			fail := true
			name := "test:sidecar-database-failure"
			if err := db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
				if fail && tx.Statement.Table == "inbounds" {
					tx.AddError(errors.New("synthetic accounting failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer db.Callback().Update().Remove(name)
			var first sidecarTrafficJournal
			svc := &service.InboundService{}
			if _, _, err := first.commit(source, owners, svc, []*coremodel.Traffic{{IsInbound: true, Tag: ib.Tag, Up: 17, Down: 23}}, []*coremodel.ClientTraffic{{Email: client.Email, Up: 11, Down: 19}}); err == nil {
				t.Fatal("expected database failure")
			}
			paths, err := filepath.Glob(filepath.Join(sidecarJournalDir(source), "*.json"))
			if err != nil || len(paths) != 1 {
				t.Fatalf("WAL not retained: %v %v", paths, err)
			}
			data, err := os.ReadFile(paths[0])
			if err != nil {
				t.Fatal(err)
			}
			var receipts int64
			if err := db.Model(&model.SidecarTrafficReceipt{}).Count(&receipts).Error; err != nil {
				t.Fatal(err)
			}
			if receipts != 0 {
				t.Fatal("receipt committed without traffic")
			}
			fail = false
			// Rename both owners and give the old names to different rows. Recovery
			// must follow the original IDs, not bill these replacement owners.
			if err := db.Model(&client).Update("email", "renamed@example.test").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Model(&ib).Update("tag", "renamed-tag").Error; err != nil {
				t.Fatal(err)
			}
			replacement := model.Inbound{Tag: "old-tag", Protocol: protocol, Settings: `{"clients":[]}`}
			if err := db.Create(&replacement).Error; err != nil {
				t.Fatal(err)
			}
			other := coremodel.ClientTraffic{Email: "old@example.test", InboundId: replacement.Id, Enable: true}
			if err := db.Create(&other).Error; err != nil {
				t.Fatal(err)
			}
			// Drop all first-job state: only durable files survive the panel replacement.
			var second sidecarTrafficJournal
			if _, _, err := second.replay(source, svc); err != nil {
				t.Fatal(err)
			}
			// Reproduce an ambiguous commit/file-removal outcome by restoring exactly
			// the same WAL; its transaction receipt must prevent a second charge.
			if err := os.WriteFile(paths[0], data, 0o600); err != nil {
				t.Fatal(err)
			}
			var third sidecarTrafficJournal
			if _, _, err := third.replay(source, svc); err != nil {
				t.Fatal(err)
			}
			if err := db.First(&client, client.Id).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&ib, ib.Id).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&other, other.Id).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.First(&replacement, replacement.Id).Error; err != nil {
				t.Fatal(err)
			}
			if client.Up != 11 || client.Down != 19 || ib.Up != 17 || ib.Down != 23 {
				t.Fatalf("wrong original totals: client=%+v inbound=%+v", client, ib)
			}
			if other.Up != 0 || other.Down != 0 || replacement.Up != 0 || replacement.Down != 0 {
				t.Fatal("old bytes charged to replacement owner")
			}
			if _, err := os.Stat(paths[0]); !os.IsNotExist(err) {
				t.Fatalf("replayed WAL remains: %v", err)
			}
		})
	}
}

func TestSidecarJournalDeletedOwnersAreNotReassigned(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	db := database.GetDB()
	ib := model.Inbound{Tag: "deleted", Protocol: model.Mieru, Settings: `{"clients":[]}`}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	client := coremodel.ClientTraffic{Email: "deleted@example.test", InboundId: ib.Id}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	owners, err := service.CaptureSidecarTrafficOwners([]model.Protocol{model.Mieru})
	if err != nil {
		t.Fatal(err)
	}
	batch := owners.Batch("deleted-owners", []*coremodel.Traffic{{Tag: ib.Tag, IsInbound: true, Up: 31}}, []*coremodel.ClientTraffic{{Email: client.Email, Down: 47}})
	if err := db.Delete(&client).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Delete(&ib).Error; err != nil {
		t.Fatal(err)
	}
	replacement := model.Inbound{Tag: ib.Tag, Protocol: model.Mieru, Settings: `{"clients":[]}`}
	if err := db.Create(&replacement).Error; err != nil {
		t.Fatal(err)
	}
	other := coremodel.ClientTraffic{Email: client.Email, InboundId: replacement.Id}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := (&service.InboundService{}).AddSidecarTrafficBatch(batch); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&other, other.Id).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&replacement, replacement.Id).Error; err != nil {
		t.Fatal(err)
	}
	if other.Down != 0 || replacement.Up != 0 {
		t.Fatal("deleted owner's usage billed to new owner")
	}
}

func TestSidecarJournalFilesystemFailureSurvivesJobReplacement(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	db := database.GetDB()
	ib := model.Inbound{Tag: "disk-failure", Protocol: model.Mieru, Settings: `{"clients":[]}`}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	client := coremodel.ClientTraffic{Email: "disk@example.test", InboundId: ib.Id, Enable: true}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}
	owners, err := service.CaptureSidecarTrafficOwners([]model.Protocol{model.Mieru})
	if err != nil {
		t.Fatal(err)
	}
	path := sidecarJournalDir("mieru")
	t.Cleanup(func() { unstoredSidecarTraffic.Delete(path) })
	// A regular file where a directory is required is a portable write failure,
	// including when the tests run as root.
	if err := os.WriteFile(path, []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}
	var first sidecarTrafficJournal
	svc := &service.InboundService{}
	if _, _, err := first.commit("mieru", owners, svc, nil, []*coremodel.ClientTraffic{{Email: client.Email, Up: 61}}); err == nil {
		t.Fatal("expected WAL failure")
	}
	if err := db.First(&client, client.Id).Error; err != nil {
		t.Fatal(err)
	}
	if client.Up != 0 {
		t.Fatal("accounting ran before WAL was durable")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	var replacement sidecarTrafficJournal
	if _, _, err := replacement.replay("mieru", svc); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&client, client.Id).Error; err != nil {
		t.Fatal(err)
	}
	if client.Up != 61 {
		t.Fatalf("lost runtime-owned pending traffic: %d", client.Up)
	}
	if _, ok := unstoredSidecarTraffic.Load(path); ok {
		t.Fatal("pending fallback was not released")
	}
}

func TestSidecarJobsRecoverJournalBeforeRuntimeSetup(t *testing.T) {
	for _, source := range []string{"mieru", "mtproto", "externalvpn"} {
		t.Run(source, func(t *testing.T) {
			dir := t.TempDir()
			t.Setenv("XUI_DB_FOLDER", dir)
			dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
			db := database.GetDB()
			row := coremodel.ClientTraffic{Email: "recovery@example.test", Enable: true}
			if err := db.Create(&row).Error; err != nil {
				t.Fatal(err)
			}
			batch := service.SidecarTrafficBatch{ID: source + "-recover", Clients: []service.SidecarTrafficDelta{{ID: row.Id, Up: 71}}}
			data, err := json.Marshal(batch)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := storeTrafficJournal(sidecarJournalDir(source), batch.ID, data); err != nil {
				t.Fatal(err)
			}
			// Recovery must precede queries for desired runtime configuration. Deliberately
			// fail those queries so no process, Docker or host firewall setup can run.
			name := "test:no-runtime-setup"
			if err := db.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
				if tx.Statement.Table == "inbounds" {
					tx.AddError(errors.New("synthetic configuration outage"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer db.Callback().Query().Remove(name)
			switch source {
			case "mieru":
				NewMieruJob().Run()
			case "mtproto":
				NewMtprotoJob().Run()
			case "externalvpn":
				NewExternalVPNJob().Run()
			}
			if err := db.First(&row, row.Id).Error; err != nil {
				t.Fatal(err)
			}
			if row.Up != 71 {
				t.Fatalf("job skipped WAL recovery: %d", row.Up)
			}
		})
	}
}
