package common

import (
	"cmp"
	"math"
)

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

// Two non-negative int64 counters fit exactly in uint64, even when their
// signed display total saturates. Preserve ordering at that boundary.
func CompareTrafficUsage(up, down, otherUp, otherDown int64) int {
	return cmp.Compare(uint64(max(up, 0))+uint64(max(down, 0)), uint64(max(otherUp, 0))+uint64(max(otherDown, 0)))
}

// Expired uses the durable millisecond cutoff; zero is unlimited and negative
// values represent delayed activation rather than an absolute deadline.
func Expired(expiryMillis, nowMillis int64) bool {
	return expiryMillis > 0 && expiryMillis <= nowMillis
}
