// Package job provides background job implementations for the 3x-ui web panel,
// including traffic monitoring, system checks, and periodic maintenance tasks.
package job

import (
	"context"

	"github.com/SawaMEN/3x-ui/v3/internal/eventbus"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// EventBus is set from web layer to publish events.
var EventBus *eventbus.Bus

// CheckXrayRunningJob monitors the selected proxy core and restarts it if it crashes.
type CheckXrayRunningJob struct {
	checkTime int
}

// NewCheckXrayRunningJob creates the core health check job instance.
func NewCheckXrayRunningJob() *CheckXrayRunningJob {
	return new(CheckXrayRunningJob)
}

// Run checks the selected core and restarts it after it is down for 2 consecutive checks.
func (j *CheckXrayRunningJob) Run() {
	native := service.NativeCore(service.CoreTypeSingBox)
	if err := native.ReconcileGateway(context.Background()); err != nil {
		logger.Warning("Gateway reconcile: ", err)
	}
	if !native.DidCrash() {
		j.checkTime = 0
		return
	}
	j.checkTime++
	if j.checkTime > 1 {
		err := native.RecoverIfCrashed(context.Background())
		j.checkTime = 0
		if err != nil {
			logger.Error("Restart sing-box failed: ", err)
		}
	}
}
