package job

import (
	"context"
	"errors"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/web/websocket"
)

type OutboundAutoBalancerJob struct{ ctx context.Context }

func NewOutboundAutoBalancerJob(parents ...context.Context) *OutboundAutoBalancerJob {
	ctx := context.Background()
	if len(parents) > 0 && parents[0] != nil {
		ctx = parents[0]
	}
	return &OutboundAutoBalancerJob{ctx: ctx}
}

func (j *OutboundAutoBalancerJob) Run() {
	ctx := j.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	changes, err := (&service.OutboundSubscriptionService{}).ProbeAutomaticSubscriptionsContext(ctx, 0)
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Warning("outbound auto balancer:", err)
	}
	if ctx.Err() != nil {
		return
	}
	applied, applyErr := (&service.OutboundSubscriptionService{}).ApplyAutomaticSelectionsContext(ctx)
	if applyErr != nil && !errors.Is(applyErr, context.Canceled) {
		logger.Warning("outbound auto balancer apply:", applyErr)
	}
	if ctx.Err() == nil && (changes > 0 || applied > 0) {
		websocket.BroadcastInvalidate(websocket.MessageTypeOutbounds)
	}
}
