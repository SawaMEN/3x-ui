package service

import (
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	DefaultDomainActivityRetentionSeconds = 300
	MinDomainActivityRetentionSeconds     = 10
	MaxDomainActivityRetentionSeconds     = 86400
	maxTrackedDomainActivity             = 3000
	maxVisibleDomainActivity             = 300
	maxVisibleDomainActivityPerClient    = 30
)

// History is bounded and private to the panel process: it is not written
// into the database or an access log. Disabling monitoring clears it.
var domainActivityHistory = struct {
	sync.Mutex
	core    string
	entries map[string]DomainActivityEntry
}{entries: make(map[string]DomainActivityEntry)}

func ValidDomainActivityRetentionSeconds(seconds int) bool {
	return seconds >= MinDomainActivityRetentionSeconds && seconds <= MaxDomainActivityRetentionSeconds
}

func domainActivityCutoff(now time.Time, retentionSeconds int) time.Time {
	if !ValidDomainActivityRetentionSeconds(retentionSeconds) {
		retentionSeconds = DefaultDomainActivityRetentionSeconds
	}
	return now.Add(-time.Duration(retentionSeconds) * time.Second)
}

// RecordDomainActivity keeps the most recent known observation for each
// (client, domain). Polling the same snapshot repeatedly never inflates its
// count. Old Xray access-log rows retain their original timestamps.
func RecordDomainActivity(core string, observed []DomainActivityEntry, now time.Time, retentionSeconds int) {
	domainActivityHistory.Lock()
	defer domainActivityHistory.Unlock()
	if domainActivityHistory.core != core {
		domainActivityHistory.core = core
		clear(domainActivityHistory.entries)
	}
	cutoff := domainActivityCutoff(now, retentionSeconds)
	for _, entry := range observed {
		if len(entry.Email) > 254 || entry.Domain == "" || entry.LastSeen.IsZero() ||
			entry.LastSeen.Before(cutoff) || entry.LastSeen.After(now.Add(10*time.Second)) {
			continue
		}
		entry.Domain = strings.ToLower(strings.TrimSpace(entry.Domain))
		if len(entry.Domain) > 253 {
			continue
		}
		key := entry.Email + "\x00" + entry.Domain
		old, ok := domainActivityHistory.entries[key]
		if !ok || entry.LastSeen.After(old.LastSeen) {
			domainActivityHistory.entries[key] = entry
		} else if entry.LastSeen.Equal(old.LastSeen) && entry.Count > old.Count {
			old.Count = entry.Count
			domainActivityHistory.entries[key] = old
		}
	}
	pruneDomainActivityLocked(cutoff)
}

func pruneDomainActivityLocked(cutoff time.Time) {
	for key, entry := range domainActivityHistory.entries {
		if entry.LastSeen.Before(cutoff) {
			delete(domainActivityHistory.entries, key)
		}
	}
	if len(domainActivityHistory.entries) <= maxTrackedDomainActivity {
		return
	}
	type order struct {
		key string
		at  time.Time
	}
	ordered := make([]order, 0, len(domainActivityHistory.entries))
	for key, entry := range domainActivityHistory.entries {
		ordered = append(ordered, order{key, entry.LastSeen})
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].at.Equal(ordered[j].at) {
			return ordered[i].key < ordered[j].key
		}
		return ordered[i].at.Before(ordered[j].at)
	})
	for _, entry := range ordered[:len(ordered)-maxTrackedDomainActivity] {
		delete(domainActivityHistory.entries, entry.key)
	}
}

// DomainActivityHistoryEntries returns the rolling site history. In-memory
// storage keeps the API fast and avoids retaining domains beyond panel restarts.
func DomainActivityHistoryEntries(core string, now time.Time, retentionSeconds int) []DomainActivityEntry {
	domainActivityHistory.Lock()
	defer domainActivityHistory.Unlock()
	if core != domainActivityHistory.core {
		clear(domainActivityHistory.entries)
		domainActivityHistory.core = core
	}
	pruneDomainActivityLocked(domainActivityCutoff(now, retentionSeconds))
	all := make([]DomainActivityEntry, 0, len(domainActivityHistory.entries))
	for _, entry := range domainActivityHistory.entries {
		all = append(all, entry)
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
	result := make([]DomainActivityEntry, 0, min(len(all), maxVisibleDomainActivity))
	perClient := map[string]int{}
	for _, entry := range all {
		if perClient[entry.Email] >= maxVisibleDomainActivityPerClient {
			continue
		}
		result = append(result, entry)
		perClient[entry.Email]++
		if len(result) >= maxVisibleDomainActivity {
			break
		}
	}
	return result
}

func ClearDomainActivityHistory() {
	domainActivityHistory.Lock()
	defer domainActivityHistory.Unlock()
	clear(domainActivityHistory.entries)
	domainActivityHistory.core = ""
}
