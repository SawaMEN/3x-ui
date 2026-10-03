package job

import (
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/web/websocket"
)

// OutboundSubscriptionJob periodically re-fetches enabled outbound subscriptions,
// updates the stored outbounds (with stable tags), and signals that xray
// should be reloaded so the new outbounds take effect.
type OutboundSubscriptionJob struct {
	subService *service.OutboundSubscriptionService
	xraySvc    *service.XrayService
}

func NewOutboundSubscriptionJob() *OutboundSubscriptionJob {
	return &OutboundSubscriptionJob{subService: &service.OutboundSubscriptionService{}, xraySvc: &service.XrayService{}}
}

func (j *OutboundSubscriptionJob) Run() {
	if err := service.SnapshotTrafficHistory(time.Now()); err != nil {
		logger.Warning("traffic history snapshot failed:", err)
	}
	if j.subService == nil { j.subService = &service.OutboundSubscriptionService{} }
	if j.xraySvc == nil { j.xraySvc = &service.XrayService{} }
	count, err := j.subService.RefreshAllEnabled()
	if err != nil { logger.Warning("outbound subscription auto-update error:", err); return }
	if count > 0 {
		logger.Infof("Refreshed %d outbound subscription(s)", count)
		core, _ := (&service.SettingService{}).GetCoreType()
		if core == service.CoreTypeSingBox { (&service.SingBoxService{}).SetToNeedRestart() } else { j.xraySvc.SetToNeedRestart() }
		websocket.BroadcastInvalidate(websocket.MessageTypeOutbounds)
	}
}
