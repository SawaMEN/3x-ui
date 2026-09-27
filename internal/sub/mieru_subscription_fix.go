package sub

import (
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// primeMieruInboundClients makes Mieru share-link generation read the same
// per-inbound settings.clients credentials that the running mita server uses.
// The normalized clients table is shared across all inbounds and may contain a
// stale/empty password after imports, while Mieru passwords are authoritative
// on each inbound.
func (s *SubService) primeMieruInboundClients(inbound *model.Inbound) {
	if inbound == nil || inbound.Protocol != model.Mieru {
		return
	}
	clients, err := s.inboundService.GetClients(inbound)
	if err != nil {
		return
	}
	s.primeLinkClients(inbound.Id, clients, true)
}

// primeMieruClientsFromSettings primes all enabled Mieru inbounds that contain
// the requested subscription identity. It is intentionally request-local: the
// SubService cache is discarded after rendering, so no credential is written
// back to the shared clients table and different Mieru inbounds may safely use
// different passwords for the same subscriber.
func (s *SubService) primeMieruClientsFromSettings(subID string) error {
	subID = strings.TrimSpace(subID)
	if subID == "" || database.GetDB() == nil {
		return nil
	}

	var inbounds []*model.Inbound
	if err := database.GetDB().
		Where("enable = ? AND protocol = ?", true, model.Mieru).
		Order("id ASC").
		Find(&inbounds).Error; err != nil {
		return err
	}

	for _, inbound := range inbounds {
		clients, err := s.inboundService.GetClients(inbound)
		if err != nil {
			continue
		}
		matched := make([]model.Client, 0, len(clients))
		for _, client := range clients {
			if strings.TrimSpace(client.SubID) == subID {
				matched = append(matched, client)
			}
		}
		if len(matched) > 0 {
			s.primeLinkClients(inbound.Id, matched, true)
		}
	}
	return nil
}
