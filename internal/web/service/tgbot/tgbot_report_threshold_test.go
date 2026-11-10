package tgbot

import "testing"

func TestClientLimitWarnEarlyThresholds(t *testing.T) {
	const now = int64(1_700_000_000_000)
	for _, tc := range []struct {
		name                string
		total, used, expiry int64
		want                bool
	}{
		{"no limits", 0, 0, 0, false},
		{"below 80 percent", 100, 79, 0, false},
		{"at 80 percent", 100, 80, 0, true},
		{"above 80 percent", 100, 95, 0, true},
		{"expires in four days", 0, 0, now + 4*86400000, false},
		{"expires in three days", 0, 0, now + 3*86400000, true},
		{"expired", 0, 0, now - 1000, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := clientLimitWarn(tc.total, tc.used, tc.expiry, now, 0, 0); got != tc.want {
				t.Fatalf("clientLimitWarn=%v, want %v", got, tc.want)
			}
		})
	}
	if !clientLimitWarn(100, 60, 0, now, 50, 0) {
		t.Fatal("previous absolute traffic threshold must remain supported")
	}
}
