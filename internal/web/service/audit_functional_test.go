package service

import (
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestAuditBulkAdjustDetachedClient(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	expiry := time.Now().Add(24 * time.Hour).UnixMilli()
	rec := model.ClientRecord{Email: "detached@audit", Enable: true, TotalGB: 1000, ExpiryTime: expiry}
	if err := database.GetDB().Create(&rec).Error; err != nil {
		t.Fatal(err)
	}
	mkTraffic(t, 0, rec.Email, 0, 0, rec.TotalGB, expiry, true)
	result, _, err := svc.BulkAdjust(&InboundService{}, []string{rec.Email}, 1, 500, "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if result.Adjusted != 1 {
		t.Fatalf("result = %+v", result)
	}
	saved, err := svc.GetRecordByEmail(nil, rec.Email)
	if err != nil {
		t.Fatal(err)
	}
	if saved.TotalGB != 1500 || saved.ExpiryTime != expiry+86400000 {
		t.Fatalf("detached record quota/expiry = %d/%d; want 1500/%d", saved.TotalGB, saved.ExpiryTime, expiry+86400000)
	}
}

func TestAuditPagingEmptyNodeSelection(t *testing.T) {
	svc, inboundSvc, settingSvc := setupPagingServices(t)
	seedPagingClients(t)
	response, err := svc.ListPaged(inboundSvc, settingSvc, ClientPageParams{Inbound: "-1"})
	if err != nil {
		t.Fatal(err)
	}
	if response.Filtered != 0 || len(response.Items) != 0 {
		t.Fatalf("empty node selection returned %d/%d clients", response.Filtered, len(response.Items))
	}
}

func TestAuditRenewalDoesNotRemainExpiredAtExactBoundary(t *testing.T) {
	const day = int64(86400000)
	for _, expiry := range []int64{day, 3 * day} {
		next, steps, err := nextIntervalRenewal(expiry, 3*day, 1, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		if next != 4*day || steps != int((3*day-expiry)/day)+1 {
			t.Errorf("renewal from %d = %d/%d, want strictly future deadline %d", expiry, next, steps, 4*day)
		}
	}
}

func TestAuditNullTemplateDoesNotPanicOrReplaceConfiguration(t *testing.T) {
	setupBulkDB(t)
	svc := &XraySettingService{}
	before, err := svc.GetXrayConfigTemplate()
	if err != nil {
		t.Fatal(err)
	}
	const want = "xray template config must be an object\n"
	if err := svc.SaveXraySetting("null"); err == nil || err.Error() != want {
		t.Fatalf("null template error = %v, want %q", err, want)
	}
	if value, err := EnsureStatsRouting("null"); value != "null" || err == nil || err.Error() != want {
		t.Fatalf("null stored template = %q/%v, want unchanged with %q", value, err, want)
	}
	after, err := svc.GetXrayConfigTemplate()
	if err != nil || before != after {
		t.Fatalf("rejected save replaced template: %v", err)
	}
}

func TestAuditEditorGroupMovePreservesHistoricalTraffic(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	seedGroupedClient(t, "group-editor@audit", "old", 100, 200)
	rec, err := svc.GetRecordByEmail(nil, "group-editor@audit")
	if err != nil {
		t.Fatal(err)
	}
	updated := rec.ToClient()
	updated.Group = "new"
	if _, err := svc.Update(&InboundService{}, rec.Id, *updated, 0); err != nil {
		t.Fatal(err)
	}
	old := groupByName(t, svc, "old")
	moved := groupByName(t, svc, "new")
	if old.TrafficUsed != 300 || old.ClientCount != 0 || moved.TrafficUsed != 0 || moved.ClientCount != 1 {
		t.Fatalf("group move transferred history: old=%+v new=%+v", old, moved)
	}
}

func TestAuditEditorDetachedClientUpdatesTrafficPolicy(t *testing.T) {
	setupBulkDB(t)
	svc := &ClientService{}
	expiry := time.Now().Add(time.Hour).UnixMilli()
	rec := model.ClientRecord{Email: "orphan-policy@audit", TotalGB: 1000, ExpiryTime: expiry, Enable: true}
	if err := database.GetDB().Create(&rec).Error; err != nil {
		t.Fatal(err)
	}
	mkTraffic(t, 0, rec.Email, 100, 200, 1000, expiry, true)
	updated := rec.ToClient()
	updated.TotalGB = 2000
	updated.ExpiryTime = expiry + 86400000
	updated.Enable = false
	updated.Reset = 7
	if _, err := svc.Update(&InboundService{}, rec.Id, *updated, 0); err != nil {
		t.Fatal(err)
	}
	stat := trafficOf(t, rec.Email)
	if stat.Total != 2000 || stat.ExpiryTime != updated.ExpiryTime || stat.Enable || stat.Reset != 7 || stat.Up != 100 || stat.Down != 200 {
		t.Fatalf("stale detached client traffic policy: %+v", stat)
	}
}
