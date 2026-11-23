package service

import (
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestLastOnlineHeartbeatCoalescesAndNeverRewinds(t *testing.T) {
	db := initTrafficTestDB(t)
	svc := &InboundService{}
	now := time.Now()
	rows := []coremodel.ClientTraffic{
		{Email: "heartbeat-recent", LastOnline: now.Add(-time.Second).UnixMilli()},
		{Email: "heartbeat-stale", LastOnline: now.Add(-time.Minute).UnixMilli()},
		{Email: "heartbeat-future", LastOnline: now.Add(time.Minute).UnixMilli()},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if err := svc.BumpClientsLastOnline([]string{rows[0].Email, rows[1].Email, rows[2].Email}); err != nil {
		t.Fatal(err)
	}
	for i, row := range rows {
		var got coremodel.ClientTraffic
		if err := db.First(&got, row.Id).Error; err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if got.LastOnline < now.UnixMilli() {
				t.Fatalf("stale heartbeat not updated: %d", got.LastOnline)
			}
		} else if got.LastOnline != row.LastOnline {
			t.Fatalf("recent or newer timestamp was overwritten: %d -> %d", row.LastOnline, got.LastOnline)
		}
	}
}

func TestTrafficPollAvoidsUnchangedInboundWrites(t *testing.T) {
	db := initTrafficTestDB(t)
	svc := &InboundService{}
	ib := model.Inbound{Tag: "idle-poll", Protocol: model.Mieru, Port: 18443, Enable: true, Settings: "{}"}
	if err := db.Create(&ib).Error; err != nil {
		t.Fatal(err)
	}
	writes := 0
	const name = "test:idle-inbound-writes"
	if err := db.Callback().Update().Before("gorm:update").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "inbounds" {
			writes++
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer db.Callback().Update().Remove(name)
	if _, _, err := svc.AddTraffic([]*coremodel.Traffic{{IsInbound: true, Tag: ib.Tag}}, nil); err != nil {
		t.Fatal(err)
	}
	if writes != 0 {
		t.Fatalf("idle poll issued %d inbound writes", writes)
	}
	if _, _, err := svc.AddTraffic([]*coremodel.Traffic{{IsInbound: true, Tag: ib.Tag, Up: 17, Down: 23}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := db.First(&ib, ib.Id).Error; err != nil {
		t.Fatal(err)
	}
	if ib.Up != 17 || ib.Down != 23 {
		t.Fatalf("lost nonzero usage: %d/%d", ib.Up, ib.Down)
	}
}
