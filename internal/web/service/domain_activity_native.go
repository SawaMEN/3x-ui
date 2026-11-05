package service

import (
	"context"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// aggregateNativeDomainActivity converts an authenticated sing-box session
// snapshot into a per-client domain summary. Only live sessions from the core
// are considered; no DNS requests, IP addresses, or URL paths are retained.
func aggregateNativeDomainActivity(sessions []singbox.ActiveSession, now time.Time) []DomainActivityEntry {
	const maxEntries = 300
	const maxPerClient = 30
	const maxSessions = 20000

	byKey := make(map[string]*DomainActivityEntry)
	seenIDs := make(map[string]struct{})
	for i, session := range sessions {
		if i >= maxSessions {
			break
		}
		user := strings.TrimSpace(session.User)
		if session.ID == "" || user == "" || len(user) > 254 {
			continue
		}
		if _, exists := seenIDs[session.ID]; exists {
			continue
		}

		// The Clash metadata Host field is usually a sniffed hostname.
		// The original destination may contain a domain when Host is empty.
		domain := domainFromAccessDestination(net.JoinHostPort(strings.TrimSpace(session.Domain), "443"))
		if domain == "" {
			domain = domainFromAccessDestination(strings.TrimSpace(session.Destination))
		}
		if domain == "" {
			continue
		}
		seenIDs[session.ID] = struct{}{}
		key := user + "\x00" + domain
		if entry := byKey[key]; entry != nil {
			entry.Count++
		} else {
			byKey[key] = &DomainActivityEntry{
				Email: user, Domain: domain, LastSeen: now, Count: 1,
			}
		}
	}
	all := make([]DomainActivityEntry, 0, len(byKey))
	for _, item := range byKey {
		all = append(all, *item)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Email != all[j].Email {
			return all[i].Email < all[j].Email
		}
		return all[i].Domain < all[j].Domain
	})
	result := make([]DomainActivityEntry, 0, min(maxEntries, len(all)))
	perClient := make(map[string]int)
	for _, item := range all {
		if perClient[item.Email] >= maxPerClient {
			continue
		}
		result = append(result, item)
		perClient[item.Email]++
		if len(result) == maxEntries {
			break
		}
	}
	return result
}

// GetNativeDomainActivity queries the selected sing-box core's live sessions.
// Both the bundled owner core and official upstream sing-box use the already
// integrated authenticated Clash API connection snapshot. It does not query
// raw traffic logs or enable a persistent collector.
func (s *ServerService) GetNativeDomainActivity(ctx context.Context, now time.Time) ([]DomainActivityEntry, error) {
	sessions, err := SelectedNativeCore().ActiveSessions(ctx)
	if err != nil {
		return nil, err
	}
	return aggregateNativeDomainActivity(sessions, now), nil
}
