package job

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
)

func TestMtprotoTrafficJobDoesNotDetachWebReconciliation(t *testing.T) {
	setupIntegrationDB(t)
	// The empty instance set must never modify host service/firewall state.
	scripts := t.TempDir()
	if err := os.WriteFile(filepath.Join(scripts, "systemctl"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", scripts+string(os.PathListSeparator)+os.Getenv("PATH"))
	entered, release, finished := make(chan struct{}), make(chan struct{}), make(chan struct{})
	cb := database.GetDB().Callback().Query()
	isWebQuery := func(tx *gorm.DB) bool {
		where, ok := tx.Statement.Clauses["WHERE"].Expression.(clause.Where)
		if !ok {
			return false
		}
		for _, expr := range where.Exprs {
			if e, ok := expr.(clause.Expr); ok && e.SQL == "protocol = ? AND node_id IS NULL AND enable = ?" {
				return true
			}
		}
		return false
	}
	if err := cb.Before("gorm:query").Register("audit:block-detached-web", func(tx *gorm.DB) {
		if isWebQuery(tx) {
			close(entered)
			<-release
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer cb.Remove("audit:block-detached-web")
	if err := cb.After("gorm:query").Register("audit:finish-detached-web", func(tx *gorm.DB) {
		if isWebQuery(tx) {
			close(finished)
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer cb.Remove("audit:finish-detached-web")
	NewMtprotoJob().Run()
	select {
	case <-entered:
		close(release)
		<-finished
		t.Fatal("traffic job returned while detached WEB reconciliation still used the database")
	case <-time.After(150 * time.Millisecond):
	}
}
