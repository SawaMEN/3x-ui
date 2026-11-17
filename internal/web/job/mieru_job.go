package job

import (
	"context"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type MieruJob struct {
	inboundService service.InboundService
	journal        sidecarTrafficJournal
}

func NewMieruJob() *MieruJob {
	return new(MieruJob)
}

func (j *MieruJob) Run() {
	// CollectTraffic advances Mieru's cumulative-counter cursors. Serialize the
	// whole poll so a slow database write cannot let another run consume a newer
	// snapshot before the previous delta has been acknowledged.
	mieruTrafficMu.Lock()
	defer mieruTrafficMu.Unlock()

	_, _, retryErr := j.journal.replay("mieru", &j.inboundService)
	if retryErr != nil {
		logger.Warning("mieru job: replay traffic journal:", retryErr)
		return
	}

	mgr := mieru.GetManager()
	desired, err := mgr.ReconcileCurrent(context.Background(), j.inboundService.DesiredMieruInstancesContext)
	if err != nil {
		logger.Warning("mieru job: get desired instances failed:", err)
		return
	}

	activeTags := make([]string, 0, len(desired))
	for _, inst := range desired {
		activeTags = append(activeTags, inst.Tag)
	}

	_, onlineEmails, collectErr := mgr.CollectTrafficPersisted(desired)
	if collectErr != nil {
		logger.Warning("mieru job: traffic poll incomplete:", collectErr)
	}
	needRestart, _, trafficErr := j.journal.replay("mieru", &j.inboundService)
	if trafficErr != nil {
		logger.Warning("mieru job: replay owned traffic:", trafficErr)
	} else if needRestart {
		if _, err := mgr.ReconcileCurrent(context.Background(), j.inboundService.DesiredMieruInstancesContext); err != nil {
			logger.Warning("mieru job: refresh desired failed:", err)
		}
	}

	if len(onlineEmails) > 0 {
		if err := j.inboundService.BumpClientsLastOnline(onlineEmails); err != nil {
			logger.Warning("mieru job: bump last online failed:", err)
		}
	}

	j.inboundService.RefreshLocalOnlineClients(onlineEmails, activeTags)
}
