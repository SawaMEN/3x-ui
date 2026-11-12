package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestRoutingCatalogCanceledWaiter(t *testing.T) {
	for _, operation := range []string{"save", "refresh", "delete"} {
		t.Run(operation, func(t *testing.T) {
			setupSettingTestDB(t)
			if err := routingListWriteMu.Acquire(context.Background(), 1); err != nil {
				t.Fatal(err)
			}
			locked := true
			defer func() {
				if locked {
					routingListWriteMu.Release(1)
				}
			}()
			ctx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() {
				switch operation {
				case "save":
					_, err := (&RoutingCatalogService{}).Save(ctx, RoutingListInput{Name: "list", URL: "https://127.0.0.1/list"})
					result <- err
				case "delete":
					result <- (&RoutingCatalogService{}).DeleteContext(ctx, 1)
				default:
					_, err := (&RoutingCatalogService{}).Refresh(ctx, 1)
					result <- err
				}
			}()
			cancel()
			select {
			case err := <-result:
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("error = %v, want cancellation", err)
				}
			case <-time.After(time.Second):
				routingListWriteMu.Release(1)
				locked = false
				<-result
				t.Fatal("canceled request waited for an unrelated catalog download")
			}
		})
	}
}

func TestRoutingCatalogCanceledRefreshPreservesSchedule(t *testing.T) {
	setupSettingTestDB(t)
	row := model.RoutingList{Name: "list", URL: "https://127.0.0.1/list?token=secret", UpdateInterval: 3600, Entries: `{"domains":["domain:example.com"]}`, LastAttempt: 123, LastError: "previous failure"}
	if err := database.GetDB().Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	changed, err := (&RoutingCatalogService{}).Refresh(ctx, row.Id)
	if changed || !errors.Is(err, context.Canceled) {
		t.Errorf("changed=%v, error=%v, want cancellation", changed, err)
	}
	var stored model.RoutingList
	if err := database.GetDB().First(&stored, row.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.LastAttempt != row.LastAttempt || stored.LastError != row.LastError || stored.Entries != row.Entries {
		t.Fatal("cancellation overwrote refresh schedule or cached result")
	}
}

func TestRoutingCatalogRefreshDueReturnsCancellation(t *testing.T) {
	setupSettingTestDB(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if changed, err := (&RoutingCatalogService{}).RefreshDue(ctx); changed || !errors.Is(err, context.Canceled) {
		t.Fatalf("changed=%v, error=%v, want cancellation", changed, err)
	}
}
