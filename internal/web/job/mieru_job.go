package job

import (
	"context"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
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
	owners, ownerErr := service.CaptureSidecarTrafficOwners([]model.Protocol{model.Mieru})
	if ownerErr != nil {
		logger.Warning("mieru job: capture traffic owners:", ownerErr)
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

	deltas, onlineEmails, collectErr := mgr.CollectTrafficChecked(desired)
	if collectErr != nil {
		logger.Warning("mieru job: traffic poll incomplete:", collectErr)
	}
	if len(deltas) > 0 || len(onlineEmails) > 0 {
		inboundByTag := make(map[string]*coremodel.Traffic)
		clientTraffic := make([]*coremodel.ClientTraffic, 0, len(deltas)+len(onlineEmails))
		seenClients := make(map[string]struct{}, len(deltas)+len(onlineEmails))

		for _, delta := range deltas {
			clientTraffic = append(clientTraffic, &coremodel.ClientTraffic{
				Email: delta.Email,
				Up:    delta.Up,
				Down:  delta.Down,
			})
			seenClients[delta.Email] = struct{}{}

			traffic := inboundByTag[delta.Tag]
			if traffic == nil {
				traffic = &coremodel.Traffic{
					IsInbound: true,
					Tag:       delta.Tag,
				}
				inboundByTag[delta.Tag] = traffic
			}
			traffic.Up = accumulateTrafficDelta(traffic.Up, delta.Up)
			traffic.Down = accumulateTrafficDelta(traffic.Down, delta.Down)
		}

		// Feed zero-byte activity for connected clients as well. This lets the
		// shared traffic layer convert delayed-start expiries on first use even
		// when the first poll observes no payload bytes.
		for _, email := range onlineEmails {
			if _, exists := seenClients[email]; exists {
				continue
			}
			clientTraffic = append(clientTraffic, &coremodel.ClientTraffic{
				Email: email,
				Up:    0,
				Down:  0,
			})
		}

		traffics := make([]*coremodel.Traffic, 0, len(inboundByTag))
		for _, traffic := range inboundByTag {
			traffics = append(traffics, traffic)
		}

		needRestart, _, trafficErr := j.journal.commit("mieru", owners, &j.inboundService, traffics, clientTraffic)
		if trafficErr != nil {
			// CollectTraffic has already advanced its cursor. Keep this exact batch
			// and retry it before the next snapshot instead of silently losing it.
			logger.Warning("mieru job: add traffic failed; batch queued for retry:", trafficErr)
		} else if needRestart {
			// Quota enforcement can disable Mieru users. Reconcile immediately
			// instead of waiting another poll for the sidecar to reload/remove them.
			if _, refreshErr := mgr.ReconcileCurrent(context.Background(), j.inboundService.DesiredMieruInstancesContext); refreshErr != nil {
				logger.Warning("mieru job: refresh desired failed:", refreshErr)
			}
		}
	}

	if len(onlineEmails) > 0 {
		if err := j.inboundService.BumpClientsLastOnline(onlineEmails); err != nil {
			logger.Warning("mieru job: bump last online failed:", err)
		}
	}

	j.inboundService.RefreshLocalOnlineClients(onlineEmails, activeTags)
}
