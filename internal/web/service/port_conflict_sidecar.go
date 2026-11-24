package service

import (
	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecaregress"
)

// The runtime generator reserves relay port numbers independently of transport
// and listen address. Mirror that conservative reservation before committing
// a listener, including disabled rows which may be enabled later.
func routedSidecarRelayPort(ib *model.Inbound, id int) int {
	if ib == nil || ib.NodeID != nil || id <= 0 {
		return 0
	}
	if routedSidecar(ib.Protocol) {
		return sidecaregress.Port(id)
	}
	if ib.Protocol == model.VKTurnProxy {
		return sidecaregress.Port(id) + 1
	}
	return 0
}

func checkRoutedSidecarRelayConflict(db *gorm.DB, inbound *model.Inbound, id int) (*portConflictDetail, error) {
	if inbound.NodeID != nil {
		return nil, nil
	}
	relay := routedSidecarRelayPort(inbound, id)
	var rows []*model.Inbound
	query := db.Model(&model.Inbound{}).Where("node_id IS NULL")
	if id > 0 {
		query = query.Where("id <> ?", id)
	}
	// For ordinary listeners only existing relay owners are relevant.
	if relay == 0 {
		query = query.Where("protocol IN ?", []model.Protocol{model.Mieru, model.TrustTunnel, model.Pingtunnel, model.VKTurnProxy})
	}
	if err := query.Find(&rows).Error; err != nil {
		return nil, err
	}
	public := inboundListenerSpans(inbound)
	// The generator also reserves the primary DB port, even for Mieru
	// whose explicit TCP/UDP bindings can be different from that port.
	if inbound.Port > 0 {
		public = append(public, listenerSpan{inbound.Port, inbound.Port, transportTCP | transportUDP})
	}
	hit := func(spans []listenerSpan, port int) bool {
		if port <= 0 {
			return false
		}
		for _, span := range spans {
			if span.first <= port && port <= span.last {
				return true
			}
		}
		return false
	}
	detail := func(row *model.Inbound, port int) *portConflictDetail {
		return &portConflictDetail{InboundID: row.Id, Tag: row.Tag, Remark: row.Remark,
			Listen: "127.0.0.1", Port: port, Relay: true, Transports: transportTCP | transportUDP}
	}
	if hit(public, relay) {
		return detail(inbound, relay), nil
	}
	if relay > 0 {
		if conflict, err := configuredLocalTCPPortConflict("127.0.0.1", []listenerSpan{{relay, relay, transportTCP}}); err != nil || conflict != nil {
			return conflict, err
		}
	}
	for _, row := range rows {
		otherRelay := routedSidecarRelayPort(row, row.Id)
		if hit(public, otherRelay) || (relay > 0 && relay == otherRelay) {
			return detail(row, otherRelay), nil
		}
		if relay == 0 {
			continue
		}
		if row.Port == relay || hit(inboundListenerSpans(row), relay) {
			return detail(row, relay), nil
		}
		if udp := independentUDPListener(row); udp != nil && udp.Port == relay {
			return detail(row, relay), nil
		}
	}
	return nil, nil
}
