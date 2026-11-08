package service

import (
	"math"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

func TestStage6QuotaDoesNotWrapOnNodeMerge(t *testing.T) {
	for _, tc := range []struct{ up, down, deltaUp, deltaDown int64 }{
		{math.MaxInt64 - 10, 20, 0, 0}, {math.MaxInt64 - 10, 0, 20, 0}, {0, math.MaxInt64 - 10, 0, 20},
	} {
		row := &coremodel.ClientTraffic{Total: math.MaxInt64, Up: tc.up, Down: tc.down}
		if masterLimitsAllowClient(row, 1, tc.deltaUp, tc.deltaDown) {
			t.Fatalf("quota bypass on overflow: %+v %+v", row, tc)
		}
	}
}

func TestStage6GroupUsageHandlesBothSaturatedCounters(t *testing.T) {
	db := initTrafficTestDB(t)
	if err := db.Create(&model.ClientRecord{Email: "saturated@stage6", Group: "large"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&coremodel.ClientTraffic{Email: "saturated@stage6", Up: database.TrafficMax, Down: database.TrafficMax}).Error; err != nil {
		t.Fatal(err)
	}
	groups, err := (&ClientService{}).ListGroups()
	if err != nil {
		t.Fatal(err)
	}
	if len(groups) != 1 || groups[0].TrafficUsed != math.MaxInt64 {
		t.Fatalf("non-negative saturated aggregate required: %+v", groups)
	}
}

func TestStage6GroupTotalsKeepIndividualCounters(t *testing.T) {
	db := stage6DB(t)
	for _, email := range []string{"one@stage6", "two@stage6"} {
		if e := db.Create(&model.ClientRecord{Email: email, Group: "large"}).Error; e != nil {
			t.Fatal(e)
		}
		if e := db.Create(&coremodel.ClientTraffic{Email: email, Up: database.TrafficMax, Down: database.TrafficMax}).Error; e != nil {
			t.Fatal(e)
		}
	}
	svc := &ClientService{}
	group := groupByName(t, svc, "large")
	if group.Up != math.MaxInt64 || group.Down != math.MaxInt64 || group.TrafficUsed != math.MaxInt64 {
		t.Fatal("aggregate did not saturate", group)
	}
	if e := svc.ResetGroupTraffic("large"); e != nil {
		t.Fatal(e)
	}
	if got := groupByName(t, svc, "large"); got.TrafficUsed != 0 {
		t.Fatal("group reset", got)
	}
	if n, e := svc.AddToGroup([]string{"one@stage6", "two@stage6"}, "next"); e != nil || n != 2 {
		t.Fatal("group move", n, e)
	}
	if got := groupByName(t, svc, "next"); got.TrafficUsed != 0 {
		t.Fatal("destination counted historical bytes", got)
	}
	var rows []coremodel.ClientTraffic
	if e := db.Find(&rows).Error; e != nil {
		t.Fatal(e)
	}
	for _, r := range rows {
		if r.Up != database.TrafficMax || r.Down != database.TrafficMax {
			t.Fatal("group operation changed durable traffic", r)
		}
	}
}
