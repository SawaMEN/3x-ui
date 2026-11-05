package tgbot

import "testing"

func TestWeeklyUsageSince(t *testing.T) {
	rows, total := weeklyUsageSince(
		map[string]int64{"bob": 100, "alice": 100},
		map[string]int64{"bob": 200, "alice": 400, "charlie": 30},
	)
	if total != 430 || len(rows) != 3 {
		t.Fatalf("bad weekly totals: total=%d, rows=%#v", total, rows)
	}
	if rows[0].Email != "alice" || rows[0].Bytes != 300 {
		t.Fatalf("wrong top client: %#v", rows)
	}
	_, total = weeklyUsageSince(map[string]int64{"reset": 500}, map[string]int64{"reset": 20})
	if total != 20 {
		t.Fatalf("reset counter must resume from zero: got %d", total)
	}
}
