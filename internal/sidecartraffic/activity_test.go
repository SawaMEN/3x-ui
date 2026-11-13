package sidecartraffic

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestOwnedZeroActivityRetainsPinnedIdentity(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	db := database.GetDB()
	ib := model.Inbound{Id: 12, Tag: "activity", Enable: true, Protocol: model.Mieru}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	original := coremodel.ClientTraffic{Email: "idle", InboundId: ib.Id, Enable: true}
	if err := db.Create(&original).Error; err != nil {
		t.Fatal(err)
	}
	owner, err := Capture("mieru", ib.Id, []string{"idle"}, false)
	if err != nil || owner == nil {
		t.Fatalf("capture: %v", err)
	}
	if err := db.Model(&original).Update("email", "retired-idle").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&coremodel.ClientTraffic{Email: "idle", InboundId: ib.Id, Enable: true}).Error; err != nil {
		t.Fatal(err)
	}
	samples := WithActivity([]Sample{{Email: "idle"}}, []string{"idle", "idle"})
	if len(samples) != 1 || !samples[0].Active {
		t.Fatalf("zero activity omitted/duplicated: %+v", samples)
	}
	if accepted, err := owner.Store(samples); !accepted || err != nil {
		t.Fatalf("store: %t %v", accepted, err)
	}
	paths, err := filepath.Glob(filepath.Join(dir, "mieru-traffic-journal", "*.json"))
	if err != nil || len(paths) != 1 {
		t.Fatalf("WAL: %v %v", paths, err)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	var batch Batch
	if err := json.Unmarshal(data, &batch); err != nil {
		t.Fatal(err)
	}
	if len(batch.Inbounds) != 0 || len(batch.Clients) != 1 || batch.Clients[0].ID != original.Id || batch.Clients[0].Up != 0 || batch.Clients[0].Down != 0 {
		t.Fatalf("wrong zero activity identity: %+v", batch)
	}
}
