package job

import (
	"path/filepath"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/dbtest"
	"github.com/SawaMEN/3x-ui/v3/internal/tuic"
)

func TestTuicAggregationPreservesCounterBounds(t *testing.T) {
	for _, identity := range []string{"traffic-id", "legacy-email", "inbound-uuid"} {
		t.Run(identity, func(t *testing.T) {
			a := tuic.ClientTrafficDelta{Email: "synthetic@example.test", Up: database.TrafficMax - 1, Down: 10}
			if identity == "traffic-id" {
				a.TrafficID = 77
			}
			if identity == "inbound-uuid" {
				a.InboundID = 7
				a.UUID = "synthetic-uuid"
			}
			b := a
			b.Up = database.TrafficMax
			b.Down = -100
			result := aggregateTuicClientTraffic([]tuic.ClientTrafficDelta{a, b}, nil)
			if len(result) != 1 || result[0].Up != database.TrafficMax || result[0].Down != 10 {
				t.Fatalf("counter aggregation overflow/subtraction: %#v", *result[0])
			}
		})
	}
}

func TestTuicShutdownJournalKeepsSaturatedCounters(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XUI_DB_FOLDER", dir)
	dbtest.InitDB(t, filepath.Join(dir, "x-ui.db"))
	db := database.GetDB()
	row := coremodel.ClientTraffic{Email: "saturated@example.test", Enable: true}
	if err := db.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	batch := tuicTrafficBatch{ID: "saturated-batch", Deltas: []tuic.ClientTrafficDelta{
		{Email: row.Email, TrafficID: row.Id, Up: database.TrafficMax - 1, Down: 10},
		{Email: row.Email, TrafficID: row.Id, Up: database.TrafficMax, Down: -100},
	}}
	j := NewTuicJob()
	for range 2 {
		if _, err := storeTuicBatch(batch); err != nil {
			t.Fatal(err)
		}
		if err := j.replayTuicJournal(); err != nil {
			t.Fatal(err)
		}
		if err := db.First(&row, row.Id).Error; err != nil {
			t.Fatal(err)
		}
		if row.Up != database.TrafficMax || row.Down != 10 {
			t.Fatalf("saturated/replayed billing: up=%d down=%d", row.Up, row.Down)
		}
	}
}
