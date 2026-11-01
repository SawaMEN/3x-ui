// Package job provides background job implementations for the 3x-ui web panel,
// including traffic monitoring, system checks, and periodic maintenance tasks.
package job

import (
	"context"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/eventbus"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// EventBus is set from web layer to publish events.
var EventBus *eventbus.Bus

const (
	coreRecoveryInitialDelay = 10 * time.Second
	coreRecoveryMaxDelay     = 2 * time.Minute
)

// coreRecoveryBackoff prevents a broken persisted configuration from starting
// a new expensive "check" process every two watchdog ticks. A successful
// recovery or an observed healthy/manually stopped core clears the backoff.
type coreRecoveryBackoff struct {
	consecutive int
	retryAfter  time.Time
	delay       time.Duration
}

func (b *coreRecoveryBackoff) shouldRecover(now time.Time) bool {
	if now.Before(b.retryAfter) {
		return false
	}
	b.consecutive++
	return b.consecutive > 1
}

func (b *coreRecoveryBackoff) failed(now time.Time) time.Duration {
	b.consecutive = 0
	if b.delay == 0 {
		b.delay = coreRecoveryInitialDelay
	} else {
		b.delay = min(b.delay*2, coreRecoveryMaxDelay)
	}
	b.retryAfter = now.Add(b.delay)
	return b.delay
}

func (b *coreRecoveryBackoff) reset() {
	*b = coreRecoveryBackoff{}
}

// CheckXrayRunningJob monitors the selected proxy core and restarts it if it crashes.
type CheckXrayRunningJob struct {
	recovery coreRecoveryBackoff
}

// NewCheckXrayRunningJob creates a new core health check job instance.
func NewCheckXrayRunningJob() *CheckXrayRunningJob {
	return new(CheckXrayRunningJob)
}

// Run checks the selected core and restarts it after it is down for 2
// consecutive checks. Repeated failures are exponentially backed off so an
// invalid config doesn't monopolize a small VPS or flood the dashboard logs.
func (j *CheckXrayRunningJob) Run() {
	if service.CoreSwitchInProgress() {
		return
	}
	native := service.SelectedNativeCore()
	if err := native.ReconcileGateway(context.Background()); err != nil {
		logger.Warning("Gateway reconcile: ", err)
	}
	if !native.DidCrash() {
		j.recovery.reset()
		return
	}
	if !j.recovery.shouldRecover(time.Now()) {
		return
	}
	err := native.RecoverIfCrashed(context.Background())
	if err != nil {
		delay := j.recovery.failed(time.Now())
		logger.Errorf("Restart sing-box failed; next automatic retry in %s: %v", delay, err)
		return
	}
	j.recovery.reset()
}
