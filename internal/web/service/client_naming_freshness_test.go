package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestSyncInboundCannotRestoreOldClientNaming(t *testing.T) {
	setupBulkDB(t)
	db := database.GetDB()
	current, stale := model.ConnectionNameClient, model.ConnectionNameRemark
	row := model.ClientRecord{Email: "shared-name@example.org", Enable: true, ConnectionNameMode: &current, UpdatedAt: 2000}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	ib := mkInbound(t, 30129, model.NaiveProxy, `{"clients":[]}`)
	old := model.Client{Email: row.Email, Enable: true, Comment: "deleted label", ConnectionNameMode: &stale, UpdatedAt: 1000}
	if err := (&ClientService{}).SyncInbound(nil, ib.Id, []model.Client{old}); err != nil {
		t.Fatal(err)
	}
	var saved model.ClientRecord
	if err := db.First(&saved, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Comment != "" || saved.ConnectionNameMode == nil || *saved.ConnectionNameMode != current {
		t.Fatalf("stale inbound restored naming: mode=%v comment=%q", saved.ConnectionNameMode, saved.Comment)
	}
	old.UpdatedAt = 3000
	old.Comment = "new label"
	if err := (&ClientService{}).SyncInbound(nil, ib.Id, []model.Client{old}); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&saved, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Comment != "new label" || saved.ConnectionNameMode == nil || *saved.ConnectionNameMode != stale {
		t.Fatal("a newer naming edit was rejected")
	}
}
