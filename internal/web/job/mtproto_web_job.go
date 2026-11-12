package job

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// MtprotoWebJob owns certificate/configuration work separately from the traffic
// poll. Run stays synchronous so cron and the server's initial-task join own it.
type MtprotoWebJob struct {
	ctx       context.Context
	mu        sync.Mutex
	reconcile func(context.Context) error
}

func NewMtprotoWebJob(parent context.Context) *MtprotoWebJob {
	if parent == nil {
		parent = context.Background()
	}
	return &MtprotoWebJob{ctx: parent, reconcile: service.ReconcileMtprotoWebProxy}
}

func (j *MtprotoWebJob) Run() {
	if !j.mu.TryLock() {
		return
	}
	defer j.mu.Unlock()
	parent := j.ctx
	if parent == nil {
		parent = context.Background()
	}
	if parent.Err() != nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, 4*time.Minute)
	defer cancel()
	reconcile := j.reconcile
	if reconcile == nil {
		reconcile = service.ReconcileMtprotoWebProxy
	}
	if err := reconcile(ctx); err != nil && !errors.Is(err, context.Canceled) {
		logger.Warning("mtproto WEB job: reconciliation failed:", err)
	}
}
