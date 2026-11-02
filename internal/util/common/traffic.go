package common

import "math"

// TrafficUsage combines non-negative byte counters without signed wraparound.
// Saturation is for totals/comparisons; each durable directional counter stays exact.
func TrafficUsage(counters ...int64) int64 {
	var total int64
	for _, n := range counters {
		if n <= 0 {
			continue
		}
		if n > math.MaxInt64-total {
			return math.MaxInt64
		}
		total += n
	}
	return total
}

func TrafficQuotaReached(limit int64, counters ...int64) bool {
	return limit > 0 && TrafficUsage(counters...) >= limit
}
