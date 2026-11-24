package adaptiveconnect

import (
	"testing"
	"time"
)

func TestFailoverAndRecovery(t *testing.T) {
	p := DefaultPolicy()
	p.Failback = true
	p.Cooldown = 10 * time.Second
	p.RecoveryThreshold = 2
	s, err := New(p, []Candidate{{"main", 0}, {"reserve", 1}})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1000, 0)
	at := func(sec int) time.Time { return start.Add(time.Duration(sec) * time.Second) }
	record := func(id string, sec int, ok bool) {
		t.Helper()
		if !s.Record(at(sec), Sample{ID: id, At: at(sec), Success: ok}) {
			t.Fatal("sample rejected")
		}
	}
	if !s.Confirm(at(0), "main") {
		t.Fatal("confirm failed")
	}
	record("reserve", 1, true)
	record("reserve", 2, true)
	record("main", 3, false)
	if d := s.Decide(at(3)); d.Target != "" {
		t.Fatal(d)
	}
	record("main", 4, false)
	if d := s.Decide(at(4)); d.Target != "" || d.Reason != "cooldown" {
		t.Fatal(d)
	}
	if d := s.Decide(at(10)); d.Target != "reserve" {
		t.Fatal(d)
	}
	// A recommendation does not alter actual state.
	if d := s.Decide(at(10)); d.Current != "main" {
		t.Fatal(d)
	}
	if !s.Confirm(at(10), "reserve") {
		t.Fatal("confirm failed")
	}
	record("main", 11, true)
	record("main", 12, true)
	record("reserve", 13, true)
	if d := s.Decide(at(13)); d.Target != "" {
		t.Fatal(d)
	}
	if d := s.Decide(at(20)); d.Target != "main" {
		t.Fatal(d)
	}
}

func TestNoUnverifiedFallback(t *testing.T) {
	p := DefaultPolicy()
	p.Cooldown = 0
	s, err := New(p, []Candidate{{"main", 0}, {"reserve", 1}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	s.Confirm(now, "main")
	s.Record(now.Add(time.Second), Sample{"main", now.Add(time.Second), false})
	s.Record(now.Add(2*time.Second), Sample{"main", now.Add(2 * time.Second), false})
	if d := s.Decide(now.Add(2 * time.Second)); d.Target != "" || d.Reason != "no-verified-alternative" {
		t.Fatal(d)
	}
	if s.Record(now, Sample{"unknown", now, true}) {
		t.Fatal("unknown accepted")
	}
	if s.Record(now, Sample{"reserve", now.Add(time.Second), true}) {
		t.Fatal("future accepted")
	}
	if s.Record(now.Add(3*time.Second), Sample{"main", now.Add(time.Second), true}) {
		t.Fatal("out-of-order accepted")
	}
}

func TestStaleSuccessNotReused(t *testing.T) {
	p := DefaultPolicy()
	p.MaxAge = 10 * time.Second
	s, err := New(p, []Candidate{{"only", 0}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	for _, sec := range []int{0, 1, 20, 21} {
		at := now.Add(time.Duration(sec) * time.Second)
		s.Record(at, Sample{"only", at, true})
	}
	if d := s.Decide(now.Add(21 * time.Second)); d.Target != "" {
		t.Fatal(d)
	}
}

func TestConfirmRejectsStaleConfirmation(t *testing.T) {
	p := DefaultPolicy()
	s, err := New(p, []Candidate{{"main", 0}, {"reserve", 1}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1000, 0)
	if !s.Confirm(now, "main") || !s.Confirm(now.Add(3*time.Second), "main") {
		t.Fatal("valid confirmation rejected")
	}
	if s.Confirm(now.Add(2*time.Second), "reserve") {
		t.Fatal("stale confirmation changed the active connection")
	}
	if d := s.Decide(now.Add(2 * time.Second)); d.Current != "main" || d.Reason != "clock-regression" {
		t.Fatal(d)
	}
	if !s.Confirm(now.Add(4*time.Second), "reserve") {
		t.Fatal("fresh confirmation rejected")
	}
	if d := s.Decide(now.Add(4 * time.Second)); d.Current != "reserve" {
		t.Fatal(d)
	}
}
