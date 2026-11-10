package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestManagerSnapshotSeparatesGlobalCountersFromFilteredResults(t *testing.T) {
	service, _, subscription := managerFixture(t)
	subscription.AutoRotate = true
	var err error
	subscription, err = service.Save(1, *subscription)
	if err != nil {
		t.Fatal(err)
	}

	manual, err := service.Save(1, model.ManagerSubscription{
		Name:            "Manual",
		Enabled:         true,
		IntervalMinutes: 60,
		Entries:         subscription.Entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	if manual.ID == subscription.ID {
		t.Fatal("test subscriptions should have distinct IDs")
	}

	queued, err := service.Enqueue(1, subscription.ID, "rotate")
	if err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&model.ManagerOperation{
		UserID: 1, SubscriptionID: manual.ID,
		State: "success", Kind: "rotate",
	}).Error; err != nil {
		t.Fatal(err)
	}

	snapshot, err := service.Snapshot(1, 1, "success", "rotate", ManagerListOptions{
		Page:    1,
		Search:  "Office",
		Compact: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.AllSubscriptionsTotal != 2 || snapshot.SubscriptionTotal != 1 {
		t.Fatalf("global vs filtered subscription totals: all=%d filtered=%d",
			snapshot.AllSubscriptionsTotal, snapshot.SubscriptionTotal)
	}
	if snapshot.AutoRotateTotal != 1 || snapshot.PendingOperations != 1 {
		t.Fatalf("global auto-rotation/pending counters: %d/%d",
			snapshot.AutoRotateTotal, snapshot.PendingOperations)
	}
	if len(snapshot.ActiveOperations) != 1 || snapshot.ActiveOperations[0].ID != queued.ID {
		t.Fatalf("active job disappeared under journal filter: %+v", snapshot.ActiveOperations)
	}
	if len(snapshot.Operations) != 1 || snapshot.Operations[0].SubscriptionID != manual.ID {
		t.Fatalf("journal filter not respected: %+v", snapshot.Operations)
	}
}
