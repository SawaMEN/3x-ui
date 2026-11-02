package common

import (
	"math"
	"math/big"
	"math/rand/v2"
	"testing"
)

func TestStage6TrafficArithmetic(t *testing.T) {
	rng := rand.New(rand.NewPCG(6, 17))
	for range 1000 {
		counters := []int64{int64(rng.Uint64()), int64(rng.Uint64()), int64(rng.Uint64()), int64(rng.Uint64())}
		sum := new(big.Int)
		for _, n := range counters {
			if n > 0 {
				sum.Add(sum, big.NewInt(n))
			}
		}
		want := int64(math.MaxInt64)
		if sum.IsInt64() {
			want = sum.Int64()
		}
		if got := TrafficUsage(counters...); got != want {
			t.Fatalf("usage %v=%d want%d", counters, got, want)
		}
		limit := int64(rng.Uint64())
		reached := limit > 0 && sum.Cmp(big.NewInt(limit)) >= 0
		if TrafficQuotaReached(limit, counters...) != reached {
			t.Fatalf("quota %v limit%d", counters, limit)
		}
	}
}
