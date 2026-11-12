package job

import (
	"github.com/SawaMEN/3x-ui/v3/internal/database"
)

// accumulateTrafficDelta merges runtime deltas without letting a counter reset
// or int64 overflow erase traffic already observed in the same polling batch.
// Runtime collectors are expected to report non-negative byte deltas; a
// negative value therefore means "ignore this sample", not "subtract bytes".
func accumulateTrafficDelta(current, delta int64) int64 {
	if current < 0 {
		current = 0
	}
	if delta <= 0 || current >= database.TrafficMax {
		return current
	}
	if delta >= database.TrafficMax-current {
		return database.TrafficMax
	}
	return current + delta
}
