package service

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestManagerHealthSwitchKeepsListenerAndRealityKeys(t *testing.T) {
	svc, inbound, sub := managerFixture(t)
	beforePort := inbound.Port
	beforeSettings := inbound.Settings
	inbound.Tag = fmt.Sprintf("manager-%s-2-0", sub.Token)
	inbound.StreamSettings = `{"network":"tcp","security":"reality","realitySettings":{"target":"primary.example.org:443","dest":"primary.example.org:443","serverNames":["primary.example.org"],"privateKey":"stable-private-key","shortIds":["ab12"]}}`
	if err := database.GetDB().Save(inbound).Error; err != nil {
		t.Fatal(err)
	}
	entry := sub.Entries[0]
	entry.ActiveInboundID = inbound.Id
	entry.SNI = "primary.example.org"
	entry.SNIBackups = []string{"backup.example.org", "third.example.org"}
	entry.SNIHealthFailures = 2
	sub.MonitorSNI = true
	sub.MonitorSNIIntervalMinutes = 10
	sub.Published = []model.ManagerEntry{entry}
	sub.Entries = []model.ManagerEntry{entry}
	if err := database.GetDB().Save(sub).Error; err != nil {
		t.Fatal(err)
	}
	op := &model.ManagerOperation{Kind: "sni_health", Total: 1}
	calls := []string{}
	err := svc.monitorSNIWithProbe(context.Background(), op, sub, func(_ context.Context, _ *model.Inbound, host string) (bool, error) {
		calls = append(calls, host)
		return host == "backup.example.org", nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []string{"primary.example.org", "backup.example.org"}) ||
		op.Completed != 1 || !strings.Contains(op.Message, "switched=1") {
		t.Fatalf("unexpected monitor pass: %v %+v", calls, op)
	}
	var after model.Inbound
	if err := database.GetDB().First(&after, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if after.Port != beforePort || after.Settings != beforeSettings || after.Tag != inbound.Tag {
		t.Fatalf("SNI switch changed listener identity or client credentials: %+v", after)
	}
	if !managerSNIHealthRealityKeysEqual(inbound.StreamSettings, after.StreamSettings) ||
		!strings.Contains(after.StreamSettings, "backup.example.org:443") {
		t.Fatalf("SNI switch rotated keys or failed to change target: %s", after.StreamSettings)
	}
	var stored model.ManagerSubscription
	if err := database.GetDB().First(&stored, sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Published[0].SNI != "backup.example.org" ||
		stored.Published[0].ActiveInboundID != inbound.Id ||
		stored.Published[0].SNIHealthFailures != 0 ||
		stored.Published[0].SNIHealthLastSwitchAt == 0 ||
		!slices.Contains(stored.Published[0].SNIBackups, "primary.example.org") {
		t.Fatalf("publication not updated safely: %+v", stored.Published)
	}
}

type managerSNIApplyOnceFailure struct {
	fakeNodeRuntime
	applyCalls atomic.Int32
}

func (r *managerSNIApplyOnceFailure) ApplyConfigNow(context.Context) error {
	if r.applyCalls.Add(1) == 1 {
		return fmt.Errorf("simulated core apply failure")
	}
	return nil
}

func TestManagerHealthSwitchRollsBackFailedCoreApply(t *testing.T) {
	svc, inbound, sub := managerFixture(t)
	inbound.Tag = fmt.Sprintf("manager-%s-3-0", sub.Token)
	inbound.StreamSettings = `{"network":"tcp","security":"reality","realitySettings":{"target":"primary.example.org:443","dest":"primary.example.org:443","serverNames":["primary.example.org"],"privateKey":"same-private-key","shortIds":["ac12"]}}`
	if err := database.GetDB().Save(inbound).Error; err != nil {
		t.Fatal(err)
	}
	entry := sub.Entries[0]
	entry.ActiveInboundID = inbound.Id
	entry.SNI = "primary.example.org"
	entry.SNIBackups = []string{"backup.example.org"}
	entry.SNIHealthFailures = 2
	sub.MonitorSNI = true
	sub.MonitorSNIIntervalMinutes = 10
	sub.Published = []model.ManagerEntry{entry}
	sub.Entries = []model.ManagerEntry{entry}
	if err := database.GetDB().Save(sub).Error; err != nil {
		t.Fatal(err)
	}
	runtime := &managerSNIApplyOnceFailure{}
	useTestRuntimeManager(t).SetLocalRuntimeOverride(runtime)
	op := &model.ManagerOperation{Kind: "sni_health", Total: 1}
	err := svc.monitorSNIWithProbe(context.Background(), op, sub, func(_ context.Context, _ *model.Inbound, host string) (bool, error) {
		return host == "backup.example.org", nil
	})
	if err == nil || !strings.Contains(err.Error(), "rolled back") || runtime.applyCalls.Load() != 2 {
		t.Fatalf("core failure did not trigger rollback: %v, apply calls=%d", err, runtime.applyCalls.Load())
	}
	var stored model.Inbound
	if err := database.GetDB().First(&stored, inbound.Id).Error; err != nil {
		t.Fatal(err)
	}
	if stored.StreamSettings != inbound.StreamSettings || stored.Port != inbound.Port ||
		stored.Settings != inbound.Settings {
		t.Fatalf("failed switch was persisted: %+v", stored)
	}
	var record model.ManagerSubscription
	if err := database.GetDB().First(&record, sub.ID).Error; err != nil {
		t.Fatal(err)
	}
	if record.Published[0].SNI != "primary.example.org" {
		t.Fatalf("failed switch changed published SNI: %+v", record.Published)
	}
}
