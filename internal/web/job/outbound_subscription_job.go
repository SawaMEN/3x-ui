package job

import (
	"context"
	"errors"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/web/websocket"
)

// OutboundSubscriptionJob refreshes subscriptions and requests a reload after changes.
type OutboundSubscriptionJob struct {
	ctx        context.Context
	subService *service.OutboundSubscriptionService
	xraySvc    *service.CoreService
}

// NewOutboundSubscriptionJob optionally binds network work to the panel lifecycle.
func NewOutboundSubscriptionJob(parents ...context.Context) *OutboundSubscriptionJob {
	ctx := context.Background()
	if len(parents) > 0 && parents[0] != nil {
		ctx = parents[0]
	}
	return &OutboundSubscriptionJob{
		ctx:        ctx,
		subService: &service.OutboundSubscriptionService{},
		xraySvc:    &service.CoreService{},
	}
}

// Run is invoked by the cron scheduler.
func (j *OutboundSubscriptionJob) Run() {
	if j.subService == nil {
		j.subService = &service.OutboundSubscriptionService{}
	}
	if j.xraySvc == nil {
		j.xraySvc = &service.CoreService{}
	}

	count, err := j.subService.RefreshAllEnabled(j.ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Warning("outbound subscription auto-update error:", err)
	}
	if count > 0 {
		logger.Infof("Refreshed %d outbound subscription(s)", count)
		// Ask the xray manager to restart/reload on the next 30s check.
		core, _ := (&service.SettingService{}).GetCoreType()
		if service.IsNativeCore(core) {
			service.SelectedNativeCore().SetToNeedRestart()
		} else {
			j.xraySvc.SetToNeedRestart()
		}
		// Also broadcast an invalidate so the UI can refresh the xray setting
		// view (new outbounds will be visible after the reload cycle).
		websocket.BroadcastInvalidate(websocket.MessageTypeOutbounds)
	}
}
