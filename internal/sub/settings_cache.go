package sub

import (
	"strconv"
	"sync"
	"time"

	"github.com/goccy/go-json"
	"golang.org/x/sync/singleflight"
)

// Subscription apps poll on multi-minute intervals, so inbound settings at most this old read the
// same as fresh ones; panel edits purge them at once.
const (
	settingsCacheTTL        = 10 * time.Second
	settingsCacheMaxEntries = 2048
	settingsCacheMaxBytes   = 8 << 20
)

// inboundSettingsCache keeps each inbound's settings without its clients array for subscription
// bodies, so every subscriber of a large inbound shares one small copy instead of the whole blob.
type inboundSettingsCache struct {
	ttl   time.Duration
	now   func() time.Time
	group singleflight.Group

	mu         sync.Mutex
	generation uint64
	entries    map[int]settingsEntry
	bytes      int
	nextSweep  time.Time
}

type settingsEntry struct {
	settings  string
	expiresAt time.Time
}

func newInboundSettingsCache(ttl time.Duration) *inboundSettingsCache {
	return &inboundSettingsCache{ttl: ttl, now: time.Now, entries: map[int]settingsEntry{}}
}

// settingsFor returns the inbound's client-less settings, loading them at most once per TTL window
// even when concurrent requests miss together.
func (c *inboundSettingsCache) settingsFor(inboundId int, load func() (string, error)) (string, error) {
	if settings, ok := c.lookup(inboundId); ok {
		return settings, nil
	}
	c.mu.Lock()
	generation := c.generation
	c.mu.Unlock()
	// A request after a panel edit must not join a pre-edit load. Keep each
	// generation's in-flight work separate as well as its stored entries.
	key := strconv.Itoa(inboundId) + ":" + strconv.FormatUint(generation, 10)
	v, err, _ := c.group.Do(key, func() (any, error) {
		if settings, ok := c.lookup(inboundId); ok {
			return settings, nil
		}
		raw, err := load()
		if err != nil {
			return "", err
		}
		settings := withoutClients(raw)
		c.mu.Lock()
		defer c.mu.Unlock()
		// A purge during the load means the row may have changed under it; serve it, never keep it.
		if generation == c.generation {
			c.storeLocked(inboundId, settings, c.now())
		}
		return settings, nil
	})
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

func (c *inboundSettingsCache) lookup(inboundId int) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[inboundId]
	if !ok {
		return "", false
	}
	if !c.now().Before(entry.expiresAt) {
		c.removeLocked(inboundId, entry)
		return "", false
	}
	return entry.settings, true
}

// Expire old settings opportunistically and bound memory even if requests
// touch thousands of different inbounds. An oversized entry is served but
// not retained.
func (c *inboundSettingsCache) storeLocked(id int, settings string, now time.Time) {
	if !now.Before(c.nextSweep) {
		for key, entry := range c.entries {
			if !now.Before(entry.expiresAt) {
				c.removeLocked(key, entry)
			}
		}
		c.nextSweep = now.Add(c.ttl)
	}
	if old, ok := c.entries[id]; ok {
		c.removeLocked(id, old)
	}
	if len(c.entries) >= settingsCacheMaxEntries || len(settings) > settingsCacheMaxBytes-c.bytes {
		return
	}
	c.entries[id] = settingsEntry{settings: settings, expiresAt: now.Add(c.ttl)}
	c.bytes += len(settings)
}

func (c *inboundSettingsCache) removeLocked(id int, entry settingsEntry) {
	delete(c.entries, id)
	c.bytes -= len(entry.settings)
}

// purgeAll drops every entry after a panel mutation, so an edit reaches the next fetch.
func (c *inboundSettingsCache) purgeAll() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.generation++
	clear(c.entries)
	c.bytes = 0
	c.nextSweep = time.Time{}
}

// withoutClients drops the clients array, nearly all of a large inbound's settings, which
// subscription bodies resolve from the clients table instead; malformed settings stay as they are.
func withoutClients(settings string) string {
	var shallow map[string]json.RawMessage
	if err := json.Unmarshal([]byte(settings), &shallow); err != nil {
		return settings
	}
	if _, ok := shallow["clients"]; !ok {
		return settings
	}
	delete(shallow, "clients")
	out, err := json.Marshal(shallow)
	if err != nil {
		return settings
	}
	return string(out)
}
