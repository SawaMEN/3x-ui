package sub

import (
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

// Pingtunnel is a single shared-secret ICMP listener, not a per-client account.
// Only emit a native client config when this subscriber was explicitly attached.
func (s *SubService) genPingtunnelLink(inbound *model.Inbound, email string) string {
	client, ok := s.clientForLink(inbound, email)
	if !ok || !client.Enable || (client.ExpiryTime > 0 && client.ExpiryTime <= time.Now().UnixMilli()) {
		return ""
	}
	inst, err := externalvpn.FromInbound(inbound)
	if err != nil {
		logger.Warningf("Subscription Pingtunnel inbound %d skipped: %v", inbound.Id, err)
		return ""
	}
	var links []string
	for _, ep := range s.shareEndpointsForInbound(inbound) {
		link, err := externalvpn.ExportPingtunnelLink(inst, ep.Address, s.endpointRemark(inbound, email, ep.ep, ""))
		if err == nil {
			links = append(links, link)
		} else {
			logger.Warningf("Subscription Pingtunnel inbound %d skipped invalid endpoint: %v", inbound.Id, err)
		}
	}
	return strings.Join(links, "\n")
}
