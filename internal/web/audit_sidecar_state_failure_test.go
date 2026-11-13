//go:build !windows

package web

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

func TestMieruReconcileStateFailurePreservesRuntime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_BIN_FOLDER", dir)
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	db := database.GetDB()
	if err := db.Create(&model.Inbound{Id: 996, Enable: true, Tag: "state-failure", Protocol: model.Mieru}).Error; err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "mita"), []byte("#!/bin/sh\ncase \"$1\" in\nget) exit 0;;\n*) exec sleep 60;;\nesac\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	mgr := mieru.GetManager()
	mgr.StopAll()
	defer mgr.StopAll()
	inst := mieru.Instance{Revision: sidecartraffic.RevisionOf(&model.Inbound{Id: 996, Enable: true, Tag: "state-failure", Protocol: model.Mieru}), Id: 996, Tag: "state-failure", Users: []mieru.User{{Name: "test", Password: "test"}}, PortBindings: []mieru.PortBinding{{Port: 34583, Protocol: "TCP"}}}
	if err := mgr.Ensure(inst); err != nil {
		t.Fatal(err)
	}
	if !mgr.HasRunning() {
		t.Fatal("fixture process not running")
	}
	if err := db.Callback().Query().Before("gorm:query").Register("audit:state-failure", func(tx *gorm.DB) { tx.AddError(errors.New("fixture read failure")) }); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove("audit:state-failure")
	// A different desired ID would remove the current process before starting the
	// replacement if authoritative validation happened after the removal loop.
	inst.Id = 997
	mgr.Reconcile([]mieru.Instance{inst})
	if !mgr.HasRunning() {
		t.Fatal("failed DB read removed healthy existing runtime")
	}
	if err := mgr.Ensure(inst); err == nil {
		t.Fatal("failed DB read permitted Ensure")
	}
	// An explicit empty desired set and Remove still work during a DB outage.
	mgr.Reconcile(nil)
	if mgr.HasRunning() {
		t.Fatal("empty desired could not stop runtime during DB outage")
	}
}
