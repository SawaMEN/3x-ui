package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
)

func TestSumTrafficDelta(t *testing.T) {
	tests := []struct {
		name string
		a    int64
		b    int64
		want int64
	}{
		{name: "negative stored repaired", a: -100, b: 20, want: 20},
		{name: "out of range stored repaired", a: database.TrafficMax + 1, b: 0, want: database.TrafficMax},
		{name: "normal", a: 10, b: 20, want: 30},
		{name: "negative delta ignored", a: 10, b: -5, want: 10},
		{name: "saturates", a: database.TrafficMax - 2, b: 10, want: database.TrafficMax},
		{name: "already saturated", a: database.TrafficMax, b: 1, want: database.TrafficMax},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sumTrafficDelta(tt.a, tt.b); got != tt.want {
				t.Fatalf("sumTrafficDelta(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
