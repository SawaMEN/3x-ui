package sub

import (
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// inboundPhysicalNodeGuid returns the stable GUID of the node that actually
// hosts an inbound. OriginNodeGuid survives chained-node adoption and therefore
// wins over the direct NodeID mapping. Local inbounds intentionally return "".
func (s *SubService) inboundPhysicalNodeGuid(inbound *model.Inbound) string {
	if inbound == nil {
		return ""
	}
	if guid := strings.TrimSpace(inbound.OriginNodeGuid); guid != "" {
		return guid
	}
	if inbound.NodeID == nil || s.nodesByID == nil {
		return ""
	}
	node := s.nodesByID[*inbound.NodeID]
	if node == nil {
		return ""
	}
	return strings.TrimSpace(node.Guid)
}

// hostAppliesToInbound enforces Host.NodeGuids at subscription render time.
// An empty scope preserves legacy behavior (all nodes, including local). A
// non-empty scope is explicit: local/unknown-node inbounds do not match it.
func (s *SubService) hostAppliesToInbound(host *model.Host, inbound *model.Inbound) bool {
	if host == nil || len(host.NodeGuids) == 0 {
		return true
	}
	guid := s.inboundPhysicalNodeGuid(inbound)
	if guid == "" {
		return false
	}
	for _, allowed := range host.NodeGuids {
		if strings.TrimSpace(allowed) == guid {
			return true
		}
	}
	return false
}
