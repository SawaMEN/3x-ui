package job

import (
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/web/service"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

type ExternalVPNJob struct{ inbounds service.InboundService }

func NewExternalVPNJob() *ExternalVPNJob { return &ExternalVPNJob{} }
func (j *ExternalVPNJob) Run() {
	desired, err := j.inbounds.DesiredExternalVPNInstances()
	if err != nil {
		logger.Warning("external VPN reconcile:", err)
		return
	}
	mgr := externalvpn.GetManager()
	mgr.Reconcile(desired)
	rows := mgr.CollectTraffic()
	inbound := map[string]*xray.Traffic{}
	clients := make([]*xray.ClientTraffic, 0, len(rows))
	activeEmails := []string{}
	activeTags := []string{}
	for _, row := range rows {
		if row.Active {
			activeEmails = append(activeEmails, row.Email)
			activeTags = append(activeTags, row.Tag)
		}
		if inbound[row.Tag] == nil {
			inbound[row.Tag] = &xray.Traffic{Tag: row.Tag, IsInbound: true}
		}
		inbound[row.Tag].Up += row.Up
		inbound[row.Tag].Down += row.Down
		clients = append(clients, &xray.ClientTraffic{Email: row.Email, Up: row.Up, Down: row.Down})
	}
	totals := make([]*xray.Traffic, 0, len(inbound))
	for _, t := range inbound {
		totals = append(totals, t)
	}
	if len(totals) > 0 || len(clients) > 0 {
		if _, disabled, err := j.inbounds.AddTraffic(totals, clients); err != nil {
			logger.Warning("external VPN traffic:", err)
		} else if disabled {
			if current, err := j.inbounds.DesiredExternalVPNInstances(); err == nil {
				mgr.Reconcile(current)
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
