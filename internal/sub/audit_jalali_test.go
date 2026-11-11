package sub

import (
	"math"
	"testing"
)

func TestAuditJalaliBaseline(t *testing.T) {
	// Frozen outputs of the original month-walking implementation, including
	// the leap boundary and dates spanning many complete cycles.
	for _, date := range [][6]int{{1970, 1, 1, 1348, 10, 11}, {2024, 3, 19, 1402, 12, 29}, {2024, 3, 20, 1403, 1, 1}, {2025, 3, 20, 1403, 12, 30}, {2025, 3, 21, 1404, 1, 1}, {2500, 12, 31, 1879, 10, 10}, {9999, 12, 31, 9378, 10, 10}} {
		y, m, d := gregorianToJalali(date[0], date[1], date[2])
		if y != date[3] || m != date[4] || d != date[5] {
			t.Errorf("%v: got %d/%d/%d", date[:3], y, m, d)
		}
	}
}

func TestAuditJalaliMaximumExpiry(t *testing.T) {
	if got := jalaliExpireDateLabel(math.MaxInt64); got == "" {
		t.Fatal("maximum positive expiry lost its label")
	}
}

func BenchmarkAuditJalaliDistantDate(b *testing.B) {
	for b.Loop() {
		gregorianToJalali(9999, 12, 31)
	}
}
