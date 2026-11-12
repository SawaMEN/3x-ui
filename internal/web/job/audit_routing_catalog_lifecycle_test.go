package job

import (
	"context"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
)

func TestRoutingCatalogJobCancelsDatabaseWork(t *testing.T) {
	setupIntegrationDB(t)
	db := database.GetDB()
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	if err := db.Callback().Query().Before("gorm:query").Register("audit:block-catalog-query", func(tx *gorm.DB) {
		if tx.Statement.Table != "routing_lists" {
			return
		}
		once.Do(func() { close(entered) })
		select {
		case <-tx.Statement.Context.Done():
			tx.AddError(tx.Statement.Context.Err())
		case <-release:
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove("audit:block-catalog-query")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	j := NewRoutingCatalogJob(ctx)
	done := make(chan struct{})
	go func() { j.Run(); close(done) }()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		<-done
		t.Fatal("catalog query not reached")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		close(release)
		<-done
		t.Fatal("catalog job ignored panel cancellation while querying the database")
	}
}

func TestRoutingCatalogJobSkipsCanceledTick(t *testing.T) {
	setupIntegrationDB(t)
	db := database.GetDB()
	queries := 0
	if err := db.Callback().Query().Before("gorm:query").Register("audit:count-canceled-tick", func(tx *gorm.DB) { queries++ }); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Query().Remove("audit:count-canceled-tick")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	NewRoutingCatalogJob(ctx).Run()
	if queries != 0 {
		t.Fatalf("stopped panel queried database %d times", queries)
	}
}
