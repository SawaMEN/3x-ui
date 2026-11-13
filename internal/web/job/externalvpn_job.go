package job

import (
	"context"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
)

type ExternalVPNJob struct {
	inbounds service.InboundService
	journal  sidecarTrafficJournal
}

func NewExternalVPNJob() *ExternalVPNJob { return &ExternalVPNJob{} }
func (j *ExternalVPNJob) Run() {
	// CollectTraffic advances the external VPN manager's cumulative-counter
	// baseline. Serialize the full poll so overlapping runs cannot consume a
	// newer snapshot while an older batch is still being committed.
	externalTrafficMu.Lock()
	defer externalTrafficMu.Unlock()

	_, _, retryErr := j.journal.replay("externalvpn", &j.inbounds)
	if retryErr != nil {
		logger.Warning("externalvpn job: replay traffic journal:", retryErr)
		return
	}
	owners, ownerErr := service.CaptureSidecarTrafficOwners([]model.Protocol{model.Pingtunnel, model.TrustTunnel, model.FPTN, model.VetroffProxy, model.OpenFlux})
	if ownerErr != nil {
		logger.Warning("externalvpn job: capture traffic owners:", ownerErr)
		return
	}

	externalvpn.RefreshOutbounds()
	mgr := externalvpn.GetManager()
	_, err := mgr.ReconcileCurrent(context.Background(), j.inbounds.DesiredExternalVPNInstancesContext)
	if err != nil {
		logger.Warning("external VPN reconcile:", err)
		return
	}

	rows, collectErr := mgr.CollectTrafficChecked()
	if collectErr != nil {
		logger.Warning("external VPN job: traffic poll incomplete:", collectErr)
	}
	inbound := map[string]*coremodel.Traffic{}
	clients := make([]*coremodel.ClientTraffic, 0, len(rows))
	activeEmails := []string{}
	activeTags := []string{}
	for _, row := range rows {
		if row.Active {
			activeEmails = append(activeEmails, row.Email)
			activeTags = append(activeTags, row.Tag)
		}
		if inbound[row.Tag] == nil {
			inbound[row.Tag] = &coremodel.Traffic{Tag: row.Tag, IsInbound: true}
		}
		inbound[row.Tag].Up = accumulateTrafficDelta(inbound[row.Tag].Up, row.Up)
		inbound[row.Tag].Down = accumulateTrafficDelta(inbound[row.Tag].Down, row.Down)
		clients = append(clients, &coremodel.ClientTraffic{Email: row.Email, Up: row.Up, Down: row.Down})
	}
	totals := make([]*coremodel.Traffic, 0, len(inbound))
	for _, t := range inbound {
		totals = append(totals, t)
	}
	if len(totals) > 0 || len(clients) > 0 {
		if _, disabled, err := j.journal.commit("externalvpn", owners, &j.inbounds, totals, clients); err != nil {
			// CollectTraffic has already advanced its baseline. Preserve this exact
			// batch and retry it before collecting a newer snapshot.
			logger.Warning("external VPN traffic; batch queued for retry:", err)
		} else if disabled {
			if _, err := mgr.ReconcileCurrent(context.Background(), j.inbounds.DesiredExternalVPNInstancesContext); err != nil {
				logger.Warning("external VPN job: refresh desired failed:", err)
			}
		}
	}
	if len(activeEmails) > 0 {
		if err := j.inbounds.BumpClientsLastOnline(activeEmails); err != nil {
			logger.Warning("external VPN online clients:", err)
		}
	}
	j.inbounds.RefreshLocalOnlineClients(activeEmails, activeTags)
}
