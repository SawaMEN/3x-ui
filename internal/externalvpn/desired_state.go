package externalvpn

import (
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

func (inst Instance) desiredState() sidecartraffic.Desired {
	clients := make([]string, 0, len(inst.Settings.Clients))
	for _, client := range inst.Settings.Clients {
		if !client.Enable {
			continue
		}
		clients = append(clients, client.Email)
	}
	return sidecartraffic.Desired{ID: inst.ID, Protocol: inst.Protocol, Revision: inst.Revision, ClientRevision: inst.ClientRevision, Clients: clients}
}
