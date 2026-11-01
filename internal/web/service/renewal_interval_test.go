package service

import (
	"math"
	"testing"
)

func TestRenewalIntervalBoundsAndAllowances(t *testing.T) {
	const day = int64(86400000)
	for _, tc := range []struct {
		name             string
		expiry, now      int64
		days, count, cap int
		want             int64
		steps            int
		invalid          bool
	}{
		{name: "catch up", expiry: day, now: day * 10000, days: 1, want: day * 10001, steps: 10000},
		{name: "partial period", expiry: day, now: day*3 + 1, days: 1, want: day * 4, steps: 3},
		{name: "cap", expiry: day, now: day * 10, days: 1, count: 2, cap: 4, want: day * 3, steps: 2},
		{name: "spent cap", expiry: day, now: day * 10, days: 1, count: 4, cap: 4, want: day},
		{name: "boundary", expiry: day, now: day, days: 1, want: day * 2, steps: 1},
		{name: "overflow period", expiry: 1, now: day, days: math.MaxInt, invalid: true},
		{name: "negative period", expiry: 1, now: day, days: -1, invalid: true},
		{name: "zero period", expiry: 1, now: day, invalid: true},
		{name: "deadline overflow", expiry: 1, now: math.MaxInt64, days: 1, want: math.MaxInt64, steps: int((math.MaxInt64-1)/day + 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, steps, err := nextIntervalRenewal(tc.expiry, tc.now, tc.days, tc.count, tc.cap)
			if tc.invalid {
				if err == nil {
					t.Fatal("invalid renewal accepted")
				}
				return
			}
			if err != nil || got != tc.want || steps != tc.steps {
				t.Fatalf("got %d/%d/%v, want %d/%d", got, steps, err, tc.want, tc.steps)
			}
		})
	}
}
