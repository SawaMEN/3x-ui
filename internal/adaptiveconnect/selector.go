// Package adaptiveconnect contains transport-independent connection selection.
// Its caller owns network I/O, probing, persistence and tunnel switching.
package adaptiveconnect

import (
	"errors"
	"sort"
	"sync"
	"time"
)

type Policy struct {
	FailureThreshold  int
	RecoveryThreshold int
	Cooldown          time.Duration
	MaxAge            time.Duration
	Failback          bool
}

func DefaultPolicy() Policy {
	return Policy{FailureThreshold: 2, RecoveryThreshold: 3, Cooldown: 30 * time.Second, MaxAge: 90 * time.Second}
}

type Candidate struct {
	ID       string
	Priority int // lower is preferred
}

type Sample struct {
	ID      string
	At      time.Time
	Success bool // must represent end-to-end data transfer, not TCP alone
}

type health struct {
	at        time.Time
	successes int
	failures  int
}

type Decision struct {
	Current string
	Target  string // empty when no change is advised
	Reason  string
}

type Selector struct {
	mu           sync.Mutex
	policy       Policy
	candidates   []Candidate
	health       map[string]health
	current      string
	lastSwitch   time.Time
	lastDecision time.Time
}

func New(policy Policy, candidates []Candidate) (*Selector, error) {
	if policy.FailureThreshold < 1 || policy.FailureThreshold > 10 ||
		policy.RecoveryThreshold < 1 || policy.RecoveryThreshold > 10 ||
		policy.Cooldown < 0 || policy.MaxAge <= 0 {
		return nil, errors.New("invalid adaptive connection policy")
	}
	if len(candidates) == 0 || len(candidates) > 256 {
		return nil, errors.New("expected 1..256 candidates")
	}
	seen := make(map[string]bool, len(candidates))
	for _, c := range candidates {
		if c.ID == "" || seen[c.ID] {
			return nil, errors.New("candidate IDs must be unique and nonempty")
		}
		seen[c.ID] = true
	}
	list := append([]Candidate(nil), candidates...)
	sort.SliceStable(list, func(i, j int) bool { return list[i].Priority < list[j].Priority })
	return &Selector{policy: policy, candidates: list, health: make(map[string]health)}, nil
}

// Record ignores duplicate, unknown, future and out-of-order observations.
func (s *Selector) Record(now time.Time, sample Sample) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.known(sample.ID) || sample.At.IsZero() || sample.At.After(now) ||
		now.Sub(sample.At) > s.policy.MaxAge {
		return false
	}
	h := s.health[sample.ID]
	if !sample.At.After(h.at) {
		return false
	}
	if !h.at.IsZero() && sample.At.Sub(h.at) > s.policy.MaxAge {
		h.successes, h.failures = 0, 0
	}
	h.at = sample.At
	if sample.Success {
		h.failures = 0
		if h.successes < s.policy.RecoveryThreshold {
			h.successes++
		}
	} else {
		h.successes = 0
		if h.failures < s.policy.FailureThreshold {
			h.failures++
		}
	}
	s.health[sample.ID] = h
	return true
}

func (s *Selector) known(id string) bool {
	for _, candidate := range s.candidates {
		if candidate.ID == id {
			return true
		}
	}
	return false
}

func (s *Selector) fresh(h health, now time.Time) bool {
	return !h.at.IsZero() && !h.at.After(now) && now.Sub(h.at) <= s.policy.MaxAge
}

// Decide only recommends a change. A caller must successfully activate and
// verify the target before acknowledging it with Confirm. Never route direct.
func (s *Selector) Decide(now time.Time) Decision {
	s.mu.Lock()
	defer s.mu.Unlock()
	d := Decision{Current: s.current, Reason: "healthy-or-awaiting-evidence"}
	if now.Before(s.lastDecision) {
		d.Reason = "clock-regression"
		return d
	}
	s.lastDecision = now
	if s.current != "" {
		h := s.health[s.current]
		failed := s.fresh(h, now) && h.failures >= s.policy.FailureThreshold
		if !failed && !s.policy.Failback {
			return d
		}
		if !s.lastSwitch.IsZero() && now.Sub(s.lastSwitch) < s.policy.Cooldown {
			d.Reason = "cooldown"
			return d
		}
		for _, c := range s.candidates {
			if c.ID == s.current {
				if !failed {
					return d
				}
				continue
			}
			other := s.health[c.ID]
			if !s.fresh(other, now) || other.successes < s.policy.RecoveryThreshold {
				continue
			}
			if !failed && (!s.fresh(h, now) || h.successes == 0) {
				return d
			}
			d.Target = c.ID
			if failed {
				d.Reason = "consecutive-failures"
			} else {
				d.Reason = "preferred-recovered"
			}
			return d
		}
		if failed {
			d.Reason = "no-verified-alternative"
		}
		return d
	}
	for _, c := range s.candidates {
		h := s.health[c.ID]
		if s.fresh(h, now) && h.successes >= s.policy.RecoveryThreshold {
			d.Target, d.Reason = c.ID, "initial-verified"
			return d
		}
	}
	d.Reason = "no-verified-alternative"
	return d
}

// Confirm records the actual active tunnel. The caller must serialize switch
// attempts and confirm only after successful activation and verification.
func (s *Selector) Confirm(now time.Time, id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.known(id) || now.IsZero() || now.Before(s.lastDecision) || now.Before(s.lastSwitch) {
		return false
	}
	if id != s.current {
		s.current, s.lastSwitch = id, now
	}
	return true
}
