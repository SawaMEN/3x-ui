package service

import (
	"path/filepath"
	"testing"

<<<<<<< HEAD
	"github.com/SawaMEN/3x-ui/v3/internal/database"
=======
	"github.com/mhsanaei/3x-ui/v3/internal/database/dbtest"
>>>>>>> 3985ba46a19406eec1a890e1842588d1956c5a10
)

// A panel restarts with an empty snapshot until the @2s ticker fires, and a
// master probing that window reads the empty answer as an offline node.
func TestCurrentStatusSamplesBeforeFirstTick(t *testing.T) {
	dbDir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dbDir)
	dbtest.InitDB(t, filepath.Join(dbDir, "x-ui.db"))

	svc := &ServerService{}
	if svc.LastStatus() != nil {
		t.Fatal("a fresh ServerService should hold no snapshot yet")
	}

	status := svc.CurrentStatus()
	if status == nil {
		t.Fatal("CurrentStatus returned nil before the first ticker run, want an on-demand sample")
	}
	if svc.LastStatus() != status {
		t.Fatal("the on-demand sample should be stored as LastStatus")
	}
	if again := svc.CurrentStatus(); again != status {
		t.Fatal("a warm CurrentStatus should reuse the stored snapshot, not resample")
	}
}
