package service

import (
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

const DomainActivityWindowSeconds = 120

// DomainActivityEntry reports a recently logged destination, not a browser tab
// or proof that the connection remains open.
type DomainActivityEntry struct {
	Email    string    `json:"email"`
	Domain   string    `json:"domain"`
	LastSeen time.Time `json:"lastSeen"`
	Count    int       `json:"count"`
}

// domainFromAccessDestination accepts only domain:port destinations. IP-only
// traffic and malformed/injected destinations are deliberately excluded.
func domainFromAccessDestination(destination string) string {
	destination = strings.TrimPrefix(destination, "tcp:")
	destination = strings.TrimPrefix(destination, "udp:")
	host, port, err := net.SplitHostPort(destination)
	if err != nil {
		return ""
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return ""
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if len(host) < 4 || len(host) > 253 || net.ParseIP(host) != nil {
		return ""
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return ""
	}
	for _, label := range labels {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return ""
		}
		for _, r := range label {
			if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
				continue
			}
			return ""
		}
	}
	return host
}

func aggregateRecentDomainActivity(logs []LogEntry, now time.Time) []DomainActivityEntry {
	const maxEntries = 300
	const maxPerClient = 30
	cutoff := now.Add(-DomainActivityWindowSeconds * time.Second)
	byKey := make(map[string]*DomainActivityEntry)
	for _, entry := range logs {
		if entry.Email == "" || len(entry.Email) > 254 || entry.Event == 1 ||
			entry.DateTime.IsZero() || entry.DateTime.Before(cutoff) || entry.DateTime.After(now.Add(10*time.Second)) {
			continue
		}
		domain := domainFromAccessDestination(entry.ToAddress)
		if domain == "" {
			continue
		}
		key := entry.Email + "\x00" + domain
		if prev, ok := byKey[key]; ok {
			prev.Count++
			if entry.DateTime.After(prev.LastSeen) {
				prev.LastSeen = entry.DateTime
			}
		} else {
			byKey[key] = &DomainActivityEntry{
				Email: entry.Email, Domain: domain, LastSeen: entry.DateTime, Count: 1,
			}
		}
	}
	all := make([]DomainActivityEntry, 0, len(byKey))
	for _, entry := range byKey {
		all = append(all, *entry)
	}
	sort.Slice(all, func(i, j int) bool {
		if !all[i].LastSeen.Equal(all[j].LastSeen) {
			return all[i].LastSeen.After(all[j].LastSeen)
		}
		if all[i].Email != all[j].Email {
			return all[i].Email < all[j].Email
		}
		return all[i].Domain < all[j].Domain
	})
	result := make([]DomainActivityEntry, 0, min(maxEntries, len(all)))
	perClient := make(map[string]int)
	for _, entry := range all {
		if perClient[entry.Email] >= maxPerClient {
			continue
		}
		result = append(result, entry)
		perClient[entry.Email]++
		if len(result) == maxEntries {
			break
		}
	}
	return result
}

// GetRecentDomainActivity reads a bounded tail of the existing Xray access log.
// It adds no database, background collector or persistent history.
func (s *ServerService) GetRecentDomainActivity(now time.Time) ([]DomainActivityEntry, error) {
	path, err := selectedXrayAccessLogPath()
	if err != nil {
		return nil, err
	}
	if path == "" || path == "none" {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("Xray access log %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("Xray access log %q is not a regular file", path)
	}
	freedoms, blackholes := s.GetDefaultLogOutboundTags()
	logs := s.GetXrayLogs("2000", "", "true", "false", "true", freedoms, blackholes)
	return aggregateRecentDomainActivity(logs, now), nil
}
