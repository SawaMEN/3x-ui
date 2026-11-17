package job

import (
	"fmt"
	"testing"
)

func TestBanSuppressionPreservesClientIdentity(t *testing.T) {
	j := &CheckClientIpJob{}
	const email = "team|one@example.com"
	banned := []IPWithTimestamp{{IP: "192.0.2.1", Timestamp: 100}}
	if got := j.filterAdvancedSinceLastBan(email, banned); len(got) != 1 {
		t.Fatalf("initial ban = %+v", got)
	}
	j.pruneBannedSeen(map[string]map[string]int64{email: {"192.0.2.1": 100}})
	if got := j.filterAdvancedSinceLastBan(email, banned); len(got) != 0 {
		t.Fatalf("live client's frozen IP was banned again: %+v", got)
	}
	j.pruneBannedSeen(nil)
	if got := j.filterAdvancedSinceLastBan(email, banned); len(got) != 1 {
		t.Fatalf("offline client retained ban suppression: %+v", got)
	}
}

func TestBanSuppressionTracksOnlyCurrentClientIPs(t *testing.T) {
	j := &CheckClientIpJob{}
	a := []IPWithTimestamp{{IP: "192.0.2.1", Timestamp: 100}}
	b := []IPWithTimestamp{{IP: "192.0.2.2", Timestamp: 100}}
	j.filterAdvancedSinceLastBan("alice", a)
	j.filterAdvancedSinceLastBan("bob", b)
	j.filterAdvancedSinceLastBan("alice", nil)
	if got := j.filterAdvancedSinceLastBan("bob", b); len(got) != 0 {
		t.Fatalf("clearing Alice also cleared Bob: %+v", got)
	}
	if got := j.filterAdvancedSinceLastBan("alice", a); len(got) != 1 {
		t.Fatalf("disappeared IP remained suppressed: %+v", got)
	}
	advanced := []IPWithTimestamp{{IP: "192.0.2.1", Timestamp: 101}}
	if got := j.filterAdvancedSinceLastBan("alice", advanced); len(got) != 1 {
		t.Fatalf("reconnected IP was suppressed: %+v", got)
	}
}

func BenchmarkBanSuppression(b *testing.B) {
	for _, clients := range []int{100, 1000, 10000} {
		b.Run(fmt.Sprint(clients), func(b *testing.B) {
			j := &CheckClientIpJob{}
			banned := []IPWithTimestamp{{IP: "192.0.2.1", Timestamp: 100}}
			for i := range clients {
				j.filterAdvancedSinceLastBan(fmt.Sprint(i), banned)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				j.filterAdvancedSinceLastBan("0", banned)
			}
		})
	}
}
