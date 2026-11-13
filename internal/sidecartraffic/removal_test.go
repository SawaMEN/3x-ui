package sidecartraffic

import (
	"errors"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestCanRemoveUsesCurrentManagerOwnership(t *testing.T) {
	dbtest.InitDB(t, filepath.Join(t.TempDir(), "x-ui.db"))
	db := database.GetDB()
	ib := model.Inbound{Id: 11, Tag: "current-removal", Protocol: model.TrustTunnel, Enable: true, Settings: `{}`}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	active := func(*model.Inbound, map[string]bool) bool { return true }
	check := func(protocols []model.Protocol, want bool) {
		t.Helper()
		got, err := CanRemove(ib.Id, protocols, active)
		if err != nil || got != want {
			t.Fatalf("remove=%t want=%t: %v", got, want, err)
		}
	}
	check([]model.Protocol{model.Mieru}, true)
	check([]model.Protocol{model.TrustTunnel, model.OpenFlux}, false)
	if err := db.Model(&ib).Update("protocol", model.OpenFlux).Error; err != nil {
		t.Fatal(err)
	}
	check([]model.Protocol{model.TrustTunnel, model.OpenFlux}, false)
	if got, err := CanRemove(ib.Id, []model.Protocol{model.OpenFlux}, func(*model.Inbound, map[string]bool) bool { return false }); err != nil || !got {
		t.Fatalf("inactive process cannot stop: %v", err)
	}
	if err := db.Callback().Query().Before("gorm:query").Register("audit:removal-failure", func(tx *gorm.DB) { tx.AddError(errors.New("private fixture details")) }); err != nil {
		t.Fatal(err)
	}
	got, err := CanRemove(ib.Id, []model.Protocol{model.OpenFlux}, active)
	if got || err == nil || err.Error() != "read current sidecar removal state failed" {
		t.Fatalf("read failure authorized removal or leaked details: %v %v", got, err)
	}
	db.Callback().Query().Remove("audit:removal-failure")
	if err := db.Model(&ib).Update("node_id", 1).Error; err != nil {
		t.Fatal(err)
	}
	check([]model.Protocol{model.OpenFlux}, true)
	if err := db.Delete(&ib).Error; err != nil {
		t.Fatal(err)
	}
	check([]model.Protocol{model.OpenFlux}, true)
}
