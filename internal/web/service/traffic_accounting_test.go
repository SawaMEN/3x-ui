package service

import (
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
)

func TestSaturatingTrafficDelta(t *testing.T) {
	tests := []struct {
		name string
		a    int64
		b    int64
		want int64
	}{
		{name: "normal", a: 10, b: 20, want: 30},
		{name: "negative delta ignored", a: 10, b: -5, want: 10},
		{name: "saturates", a: database.TrafficMax - 2, b: 10, want: database.TrafficMax},
		{name: "already saturated", a: database.TrafficMax, b: 1, want: database.TrafficMax},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := saturatingTrafficDelta(tt.a, tt.b); got != tt.want {
				t.Fatalf("saturatingTrafficDelta(%d, %d) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
