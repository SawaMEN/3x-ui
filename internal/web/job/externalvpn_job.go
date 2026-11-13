package job

import (
	"context"

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

	externalvpn.RefreshOutbounds()
	mgr := externalvpn.GetManager()
	_, err := mgr.ReconcileCurrent(context.Background(), j.inbounds.DesiredExternalVPNInstancesContext)
	if err != nil {
		logger.Warning("external VPN reconcile:", err)
		return
	}

	rows, collectErr := mgr.CollectTrafficPersisted()
	if collectErr != nil {
		logger.Warning("external VPN job: traffic poll incomplete:", collectErr)
	}
	activeEmails := []string{}
	activeTags := []string{}
	for _, row := range rows {
		if row.Active {
			activeEmails = append(activeEmails, row.Email)
			activeTags = append(activeTags, row.Tag)
		}
	}
	_, disabled, trafficErr := j.journal.replay("externalvpn", &j.inbounds)
	if trafficErr != nil {
		logger.Warning("external VPN job: replay owned traffic:", trafficErr)
	} else if disabled {
		if _, err := mgr.ReconcileCurrent(context.Background(), j.inbounds.DesiredExternalVPNInstancesContext); err != nil {
			logger.Warning("external VPN job: refresh desired failed:", err)
		}
	}

	if len(activeEmails) > 0 {
		if err := j.inbounds.BumpClientsLastOnline(activeEmails); err != nil {
			logger.Warning("external VPN online clients:", err)
		}
	}
	j.inbounds.RefreshLocalOnlineClients(activeEmails, activeTags)
}
