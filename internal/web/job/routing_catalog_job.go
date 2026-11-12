package job

import (
	"context"
	"errors"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type RoutingCatalogJob struct {
	mu  sync.Mutex
	ctx context.Context
}

func NewRoutingCatalogJob(parents ...context.Context) *RoutingCatalogJob {
	ctx := context.Background()
	if len(parents) > 0 && parents[0] != nil {
		ctx = parents[0]
	}
	return &RoutingCatalogJob{ctx: ctx}
}

func (j *RoutingCatalogJob) Run() {
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
	changed, err := (&service.RoutingCatalogService{}).RefreshDue(ctx)
	if changed && ctx.Err() == nil {
		service.MarkRoutingCatalogChanged()
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		logger.Warning("routing catalog update: ", err)
	}
}
