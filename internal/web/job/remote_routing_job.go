package job

import (
	"context"
	"errors"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/sub"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// RemoteRoutingJob keeps remote Happ and Clash/Mihomo routing URLs warm: all
// network work runs here (cron + startup warm), never in a request handler.
type RemoteRoutingJob struct {
	ctx            context.Context
	mu             sync.Mutex
	settingService service.SettingService
}

func NewRemoteRoutingJob(parents ...context.Context) *RemoteRoutingJob {
	ctx := context.Background()
	if len(parents) > 0 && parents[0] != nil {
		ctx = parents[0]
	}
	return &RemoteRoutingJob{ctx: ctx}
}

func (j *RemoteRoutingJob) Run() {
	if !j.mu.TryLock() {
		return
	}
	defer j.mu.Unlock()
	ctx := j.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if ctx.Err() != nil {
		return
	}
	happ, err := j.settingService.GetSubRoutingRules()
	if err != nil {
		logger.Warning("Could not read Happ routing source:", err)
		return
	}
	clash, err := j.settingService.GetSubClashRules()
	if err != nil {
		logger.Warning("Could not read Clash routing source:", err)
		return
	}
	jsonRouting, err := j.settingService.GetSubJsonRoutingRules()
	if err != nil {
		logger.Warning("Could not read JSON subscription routing source:", err)
		return
	}
	if err := sub.RefreshRemoteRoutingSourcesContext(ctx, happ, clash, jsonRouting); err != nil && !errors.Is(err, context.Canceled) {
		logger.Warning("Remote routing refresh failed:", err)
	}
}
