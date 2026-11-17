package mtproto

import (
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

func (inst Instance) desiredState() sidecartraffic.Desired {
	clients := make([]string, 0, len(inst.Secrets))
	for _, client := range inst.Secrets {
		clients = append(clients, client.Name)
	}
	return sidecartraffic.Desired{ID: inst.Id, Protocol: model.MTProto, Revision: inst.Revision, ClientRevision: inst.ClientRevision, Clients: clients}
}
