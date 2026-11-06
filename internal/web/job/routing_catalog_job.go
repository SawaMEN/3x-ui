package job

import (
	"context"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type RoutingCatalogJob struct{ mu sync.Mutex }

func NewRoutingCatalogJob() *RoutingCatalogJob { return &RoutingCatalogJob{} }

func (j *RoutingCatalogJob) Run() {
	if !j.mu.TryLock() {
		return
	}
	defer j.mu.Unlock()
	changed, err := (&service.RoutingCatalogService{}).RefreshDue(context.Background())
	if changed {
		service.MarkRoutingCatalogChanged()
	}
	if err != nil {
		logger.Warning("routing catalog update: ", err)
	}
}
