package service

import (
	"fmt"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestManagerBulkSharedCreatesIndependentClientsOnOneInbound(t *testing.T) {
	svc, existing, _ := managerFixture(t)
	out, err := svc.BulkCreateShared(1, ManagerBulkSharedInput{
		NamePrefix: "Member", Count: 3, InboundIDs: []int{existing.Id},
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.Created != 3 || out.Queued != 3 || len(out.IDs) != 3 {
		t.Fatalf("unexpected result: %+v", out)
	}
	seen := map[string]bool{}
	for i, id := range out.IDs {
		var sub model.ManagerSubscription
		if err := database.GetDB().Where("id = ? AND user_id = ?", id, 1).First(&sub).Error; err != nil {
			t.Fatal(err)
		}
		if sub.Name != fmt.Sprintf("Member %03d", i+1) || seen[sub.Token] {
			t.Fatalf("missing unique identity: %+v", sub)
		}
		seen[sub.Token] = true
		if len(sub.Entries) != 1 || sub.Entries[0].Mode != "existing" || sub.Entries[0].TemplateID != existing.Id {
			t.Fatalf("bulk shared template mismatch: %+v", sub)
		}
		if op := managerRun(t, svc, &sub); op.State != "success" {
			t.Fatalf("client %d: %s", i, op.Message)
		}
		if sub.ClientID <= 0 || !sub.OwnClient {
			t.Fatalf("client not created: %+v", sub)
		}
		var attached int64
		if err := database.GetDB().Model(&model.ClientInbound{}).Where("client_id = ? AND inbound_id = ?", sub.ClientID, existing.Id).Count(&attached).Error; err != nil || attached != 1 {
			t.Fatalf("client not attached to shared inbound: %d, %v", attached, err)
		}
	}
	var count int64
	if err := database.GetDB().Model(&model.Inbound{}).Where("id = ?", existing.Id).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("shared inbound was replaced: %d, %v", count, err)
	}
}

func TestManagerBulkSharedIsAtomicAndOwnerScoped(t *testing.T) {
	svc, existing, _ := managerFixture(t)
	var before int64
	if err := database.GetDB().Model(&model.ManagerSubscription{}).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	bad := []ManagerBulkSharedInput{
		{NamePrefix: "x", Count: 0, InboundIDs: []int{existing.Id}},
		{NamePrefix: "x", Count: 101, InboundIDs: []int{existing.Id}},
		{NamePrefix: "\ninjected", Count: 1, InboundIDs: []int{existing.Id}},
		{NamePrefix: "user\rname", Count: 1, InboundIDs: []int{existing.Id}},
		{NamePrefix: "user\n", Count: 1, InboundIDs: []int{existing.Id}},
		{NamePrefix: "x", Count: 2, InboundIDs: []int{existing.Id, existing.Id}},
		{NamePrefix: "x", Count: 2, InboundIDs: []int{existing.Id, 99999}},
		{NamePrefix: "x", Count: 2, InboundIDs: nil},
	}
	for _, input := range bad {
		if _, err := svc.BulkCreateShared(1, input); err == nil {
			t.Fatalf("invalid bulk request accepted: %+v", input)
		}
	}
	if _, err := svc.BulkCreateShared(2, ManagerBulkSharedInput{NamePrefix: "x", Count: 1, InboundIDs: []int{existing.Id}}); err == nil {
		t.Fatal("other owner could create clients on a foreign inbound")
	}
	var after int64
	if err := database.GetDB().Model(&model.ManagerSubscription{}).Count(&after).Error; err != nil || after != before {
		t.Fatalf("bulk validation inserted rows: %d -> %d, %v", before, after, err)
	}
}

func TestManagerBulkSharedRejectsInactiveAndManagerOwnedInbounds(t *testing.T) {
	svc, existing, _ := managerFixture(t)
	originalTag := existing.Tag
	existing.Tag = "manager-test-owned"
	if err := database.GetDB().Save(existing).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BulkCreateShared(1, ManagerBulkSharedInput{NamePrefix: "User", Count: 1, InboundIDs: []int{existing.Id}}); err == nil {
		t.Fatal("manager-generated inbound accepted as a shared pool")
	}
	existing.Tag, existing.Enable = originalTag, false
	if err := database.GetDB().Save(existing).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := svc.BulkCreateShared(1, ManagerBulkSharedInput{NamePrefix: "User", Count: 1, InboundIDs: []int{existing.Id}}); err == nil {
		t.Fatal("disabled inbound accepted")
	}
}
