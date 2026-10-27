package service

import (
	"fmt"
	"math"
)

// nextIntervalRenewal spends one allowance for every missed period without
// iterating through them or overflowing duration, deadline or count arithmetic.
func nextIntervalRenewal(expiry, now int64, days, count, cap int) (int64, int, error) {
	if err := validateClientReset(days); err != nil {
		return expiry, 0, err
	}
	if days == 0 {
		return expiry, 0, fmt.Errorf("renewal interval must be positive")
	}
	if expiry <= 0 {
		return expiry, 0, fmt.Errorf("renewal deadline must be positive")
	}
	if expiry >= now {
		return expiry, 0, nil
	}
	period := int64(days) * 86400000
	difference := now - expiry
	steps := difference / period
	if difference%period != 0 {
		steps++
	}
	count = max(count, 0)
	if cap > 0 {
		steps = min(steps, int64(max(cap-count, 0)))
	}
	if steps == 0 {
		return expiry, 0, nil
	}
	if steps > int64(math.MaxInt) {
		return expiry, 0, fmt.Errorf("too many renewal intervals")
	}
	if steps > (math.MaxInt64-expiry)/period {
		return math.MaxInt64, int(steps), nil
	}
	return expiry + steps*period, int(steps), nil
}
