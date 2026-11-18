package job

import (
	"context"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

// DomainActivityJob maintains the operator-enabled rolling domain history
// even when no browser has the activity dialog open. A TryLock prevents
// overlapping polls while Clash API reconnects during a core restart.
type DomainActivityJob struct {
	mu       sync.Mutex
	settings service.SettingService
	server   service.ServerService
}

func NewDomainActivityJob() *DomainActivityJob { return &DomainActivityJob{} }

func (j *DomainActivityJob) Run() {
	if !j.mu.TryLock() {
		return
	}
	defer j.mu.Unlock()

	enabled, err := j.settings.GetDomainActivityEnabled()
	if err != nil {
		return
	}
	if !enabled {
		service.ClearDomainActivityHistory()
		return
	}
	retention, err := j.settings.GetDomainActivityRetentionSeconds()
	if err != nil {
		return
	}
	core, err := j.settings.GetCoreType()
	if err != nil {
		return
	}

	generation := service.DomainActivityHistoryGeneration()
	now := time.Now()
	var entries []service.DomainActivityEntry
	switch core {
	case service.CoreTypeSingBox, service.CoreTypeSingBoxUpstream:
		if !service.SelectedNativeCore().IsRunning() {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		entries, err = j.server.GetNativeDomainActivity(ctx, now)
	case service.CoreTypeXray:
		accessEnabled, accessErr := j.settings.GetAccessLogEnable()
		if accessErr != nil || !accessEnabled {
			return
		}
		entries, err = j.server.GetRecentDomainActivity(now)
	default:
		service.ClearDomainActivityHistory()
		return
	}
	if err != nil {
		return
	}
	// A core switch may complete while a slow API call is still in progress.
	// Never attribute a response from the previous core to the new history.
	currentCore, currentErr := j.settings.GetCoreType()
	if currentErr != nil || currentCore != core {
		return
	}
	service.RecordDomainActivityIfGeneration(core, entries, now, retention, generation)
}
