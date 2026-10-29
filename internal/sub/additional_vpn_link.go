package sub

import (
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
)

func (s *SubService) genAdditionalVPNLink(ib *model.Inbound, email string) string {
	client, ok := s.clientForLink(ib, email)
	if !ok || ib.Protocol == model.VetroffProxy && (!client.Enable || client.ExpiryTime > 0 && client.ExpiryTime <= time.Now().UnixMilli()) {
		return ""
	}
	inst, err := externalvpn.FromInbound(ib)
	if err != nil {
		return ""
	}
	var links []string
	for _, ep := range s.shareEndpointsForInbound(ib) {
		link, err := externalvpn.ExportAdditionalLink(inst, email, ep.Address, ep.Port, s.endpointRemark(ib, email, ep.ep, ""))
		if err == nil {
			links = append(links, link)
		}
	}
	return strings.Join(links, "\n")
}
