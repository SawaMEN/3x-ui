package service

import (
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
)

func TestManagerHealthMonitoringRequiresBackupsAndIsOwnerScoped(t *testing.T) {
	svc, inbound, sub := managerFixture(t)
	sub.MonitorSNI = true
	if _, err := svc.Save(1, *sub); err == nil {
		t.Fatal("REALITY health monitor accepted a subscription without backup sites")
	}
	inbound.StreamSettings = `{"network":"tcp","security":"reality","realitySettings":{"target":"primary.example.org:443","serverNames":["primary.example.org"]}}`
	if err := database.GetDB().Save(inbound).Error; err != nil {
		t.Fatal(err)
	}
	sub.Entries[0].SNI = "primary.example.org"
	sub.Entries[0].SNIBackups = []string{"backup.example.org"}
	updated, err := svc.Save(1, *sub)
	if err != nil {
		t.Fatal(err)
	}
	if !updated.MonitorSNI || updated.MonitorSNIIntervalMinutes != 10 ||
		updated.NextSNIHealthCheck <= time.Now().UnixMilli() {
		t.Fatalf("health schedule missing or not normalized: %+v", updated)
	}
	if _, err := svc.Enqueue(2, updated.ID, "sni_health"); err == nil {
		t.Fatal("another owner could schedule a health operation")
	}
	op, err := svc.Enqueue(1, updated.ID, "sni_health")
	if err != nil || op.Kind != "sni_health" {
		t.Fatalf("health monitor was not enqueued: %+v %v", op, err)
	}
	again, err := svc.Enqueue(1, updated.ID, "sni_health")
	if err != nil || again.ID != op.ID {
		t.Fatal("duplicate health operation was not deduplicated")
	}
}
