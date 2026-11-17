package job

import (
	"testing"
	"time"
)

func TestCoreRecoveryBackoff(t *testing.T) {
	var b coreRecoveryBackoff
	now := time.Unix(1_000, 0)

	if b.shouldRecover(now) {
		t.Fatal("first failed health check should not restart the core")
	}
	if !b.shouldRecover(now.Add(3 * time.Second)) {
		t.Fatal("second failed health check should attempt a restart")
	}

	for _, want := range []time.Duration{
		10 * time.Second, 20 * time.Second, 40 * time.Second,
		80 * time.Second, 2 * time.Minute, 2 * time.Minute,
	} {
		delay := b.failed(now)
		if delay != want {
			t.Fatalf("retry delay = %s, want %s", delay, want)
		}
		if b.shouldRecover(now.Add(delay - time.Nanosecond)) {
			t.Fatal("retry occurred before the backoff interval elapsed")
		}
		now = now.Add(delay)
		if b.shouldRecover(now) {
			t.Fatal("first health check after cooldown must not restart")
		}
		if !b.shouldRecover(now.Add(3 * time.Second)) {
			t.Fatal("second health check after cooldown should restart")
		}
		now = now.Add(3 * time.Second)
	}

	b.reset()
	if b.delay != 0 || !b.retryAfter.IsZero() {
		t.Fatal("successful recovery must clear the retry cooldown")
	}
	if b.shouldRecover(now) {
		t.Fatal("successful recovery must reset consecutive checks")
	}
	if !b.shouldRecover(now.Add(3 * time.Second)) {
		t.Fatal("new crash must still trigger after two checks")
	}
	if got := b.failed(now); got != coreRecoveryInitialDelay {
		t.Fatalf("next crash should start fresh backoff, got %s", got)
	}
}

func TestCoreRecoveryBackoffResetOnHealthyCore(t *testing.T) {
	b := coreRecoveryBackoff{}
	now := time.Unix(1_000, 0)
	b.failed(now)
	b.reset() // Called when the process runs or an operator stops it.
	if b.shouldRecover(now) {
		t.Fatal("healthy state should clear the crash streak")
	}
	if !b.shouldRecover(now.Add(3 * time.Second)) {
		t.Fatal("new crash should be retried without waiting for an old cooldown")
	}
}

func TestCoreRecoveryBackoffSurvivesTrafficCrashAfterSuccessfulStart(t *testing.T) {
	var b coreRecoveryBackoff
	now := time.Unix(1000, 0)
	b.recovered(now)
	b.healthy(now.Add(time.Second))
	if b.shouldRecover(now.Add(2 * time.Second)) {
		t.Fatal("healthy tick cleared cooldown")
	}
	if b.shouldRecover(now.Add(10 * time.Second)) {
		t.Fatal("first failed observation restarted")
	}
	if !b.shouldRecover(now.Add(13 * time.Second)) {
		t.Fatal("second failed observation did not retry")
	}
	b.recovered(now.Add(13 * time.Second))
	if b.delay != 20*time.Second {
		t.Fatalf("runtime crash did not back off: %s", b.delay)
	}
	b.healthy(now.Add(14 * time.Second))
	if b.delay == 0 {
		t.Fatal("one healthy tick reset backoff")
	}
	b.healthy(now.Add(44 * time.Second))
	if b.delay != 0 {
		t.Fatal("stable runtime did not reset backoff")
	}
}
