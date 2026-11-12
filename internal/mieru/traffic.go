package mieru

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultOnlineGrace = 120 * time.Second

type TrafficDelta struct {
	Tag   string
	Email string
	Up    int64
	Down  int64
}

type trafficCursor struct {
	up          int64
	down        int64
	initialized bool
}

type userTrafficStats struct {
	up         int64
	down       int64
	lastActive time.Time
}

// CollectTraffic reads per-user rolling 24-hour counters from every running
// mita instance. The counters are cumulative within their rolling window, so
// only high-water deltas are returned. A process restart or counter/window
// reset initializes a new cursor rather than creating a false traffic spike.
func (m *Manager) CollectTraffic(desired []Instance) ([]TrafficDelta, []string) {
	deltas, online, _ := m.CollectTrafficChecked(desired)
	return deltas, online
}

// CollectTrafficChecked preserves successful samples and reports unavailable or
// replaced sources separately from a successful empty poll.
func (m *Manager) CollectTrafficChecked(desired []Instance) ([]TrafficDelta, []string, error) {
	m.collectMu.Lock()
	defer m.collectMu.Unlock()
	var collectErr error
	now := time.Now()
	onlineSet := make(map[string]struct{})
	deltas := make([]TrafficDelta, 0)

	type scrapeResult struct {
		inst  Instance
		stats map[string]userTrafficStats
		err   error
		owner *managed
	}

	results := make(chan scrapeResult, len(desired))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, inst := range desired {
		m.mu.Lock()
		owner := m.procs[inst.Id]
		running := owner != nil && owner.proc != nil && owner.proc.IsRunning()
		m.mu.Unlock()
		if !running {
			if owner != nil {
				collectErr = errors.Join(collectErr, fmt.Errorf("mieru inbound %d: process is not running", inst.Id))
			}
			continue
		}
		wg.Add(1)
		go func(inst Instance, owner *managed) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			stats, err := scrapeUsersChecked(socketPathForID(inst.Id), configPathForID(inst.Id))
			results <- scrapeResult{inst: inst, stats: stats, err: err, owner: owner}
		}(inst, owner)
	}
	wg.Wait()
	close(results)

	for result := range results {
		if result.err != nil {
			collectErr = errors.Join(collectErr, fmt.Errorf("mieru inbound %d: %w", result.inst.Id, result.err))
			continue
		}
		m.mu.Lock()
		if m.procs[result.inst.Id] != result.owner {
			m.mu.Unlock()
			collectErr = errors.Join(collectErr, fmt.Errorf("mieru inbound %d: process changed during metrics poll", result.inst.Id))
			continue
		}
		cursors := m.traffic[result.inst.Id]
		if cursors == nil {
			cursors = make(map[string]trafficCursor)
			m.traffic[result.inst.Id] = cursors
		}
		instanceDeltas, instanceOnline := collectInstanceTraffic(result.inst, cursors, result.stats, now)
		deltas = append(deltas, instanceDeltas...)
		for _, email := range instanceOnline {
			onlineSet[email] = struct{}{}
		}
		m.mu.Unlock()
	}

	online := make([]string, 0, len(onlineSet))
	for email := range onlineSet {
		online = append(online, email)
	}
	return deltas, online, collectErr
}

// CollectRunningTraffic samples the actual running users, including inbounds
// disabled in the DB since the last periodic poll. It never reconciles processes.
func (m *Manager) CollectRunningTraffic() ([]TrafficDelta, []string) {
	deltas, online, _ := m.CollectRunningTrafficChecked()
	return deltas, online
}

func (m *Manager) CollectRunningTrafficChecked() ([]TrafficDelta, []string, error) {
	m.mu.Lock()
	instances := make([]Instance, 0, len(m.procs))
	for id, cur := range m.procs {
		instances = append(instances, Instance{Id: id, Tag: cur.tag, Users: slices.Clone(cur.users)})
	}
	m.mu.Unlock()
	return m.CollectTrafficChecked(instances)
}

// collectInstanceTraffic advances the cumulative per-user cursors for one
// Mieru instance. A successful scrape is not guaranteed to contain every
// configured user, so cursors are retained for configured users that are
// temporarily absent from the output. This prevents traffic accrued while a
// user is omitted from one or more scrapes from being silently discarded when
// that user reappears.
func collectInstanceTraffic(inst Instance, cursors map[string]trafficCursor, stats map[string]userTrafficStats, now time.Time) ([]TrafficDelta, []string) {
	configured := make(map[string]struct{}, len(inst.Users))
	for _, user := range inst.Users {
		email := strings.TrimSpace(user.Name)
		if email != "" {
			configured[email] = struct{}{}
		}
	}

	deltas := make([]TrafficDelta, 0, len(stats))
	online := make([]string, 0, len(stats))
	for user, st := range stats {
		email := strings.TrimSpace(user)
		if email == "" {
			continue
		}
		if _, ok := configured[email]; !ok {
			continue
		}

		cur := cursors[email]
		var delta TrafficDelta
		if cur.initialized {
			if st.up > cur.up {
				delta.Up = st.up - cur.up
			}
			if st.down > cur.down {
				delta.Down = st.down - cur.down
			}
		}
		cur.up = st.up
		cur.down = st.down
		cur.initialized = true
		cursors[email] = cur

		if delta.Up > 0 || delta.Down > 0 {
			delta.Tag = inst.Tag
			delta.Email = email
			deltas = append(deltas, delta)
		}
		if !st.lastActive.IsZero() && now.Sub(st.lastActive) <= defaultOnlineGrace {
			online = append(online, email)
		}
	}

	// Only configuration removal invalidates a cursor. Temporary absence from a
	// successful `get users` response must preserve the last cumulative sample.
	for email := range cursors {
		if _, ok := configured[email]; !ok {
			delete(cursors, email)
		}
	}
	return deltas, online
}

func scrapeUsersChecked(socketPath, configPath string) (map[string]userTrafficStats, error) {
	bin := GetBinaryPath()
	if _, err := os.Stat(bin); err != nil {
		return nil, errors.New("user metrics unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	absSocket, err := filepath.Abs(socketPath)
	if err != nil {
		return nil, errors.New("user metrics unavailable")
	}
	absConfig, err := filepath.Abs(configPath)
	if err != nil {
		return nil, errors.New("user metrics unavailable")
	}

	cmd := exec.CommandContext(ctx, bin, "get", "users")
	cmd.Env = append(os.Environ(),
		"MITA_CONFIG_JSON_FILE="+absConfig,
		"MITA_UDS_PATH="+absSocket,
		"MITA_INSECURE_UDS=true",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.New("user metrics unavailable")
	}
	return parseUsersTable(string(out))
}

const userBytePattern = `(?:-|[0-9]+(?:\.[0-9]+)?\s*(?:B|KiB|MiB|GiB|TiB|PiB))`

var userStatsRow = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(` + userBytePattern + `)\s+(` + userBytePattern + `)(?:\s|$)`)

func parseUsersTable(output string) (map[string]userTrafficStats, error) {
	result := make(map[string]userTrafficStats)
	recognized := false
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		// Pinned mita prints this header even for a legitimate empty snapshot.
		if len(fields) >= 4 && fields[0] == "User" && fields[1] == "LastActive" {
			recognized = true
			continue
		}
		match := userStatsRow.FindStringSubmatch(line)
		if match == nil {
			return nil, errors.New("invalid user metrics response")
		}
		stats := userTrafficStats{}
		if match[2] != "-" {
			ts, err := time.Parse(time.RFC3339, match[2])
			if err != nil {
				return nil, errors.New("invalid user metrics timestamp")
			}
			stats.lastActive = ts
		}
		stats.down = parseByteToken(strings.Join(strings.Fields(match[3]), ""))
		stats.up = parseByteToken(strings.Join(strings.Fields(match[4]), ""))
		if stats.down < 0 || stats.up < 0 {
			return nil, errors.New("invalid user metrics counter")
		}
		result[match[1]] = stats
		recognized = true
	}
	if !recognized {
		return nil, errors.New("invalid user metrics response")
	}
	return result, nil
}

func parseByteToken(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" {
		return 0
	}
	i := 0
	for i < len(value) {
		c := value[i]
		if (c >= '0' && c <= '9') || c == '.' || c == '-' {
			i++
			continue
		}
		break
	}
	if i == 0 || i == len(value) {
		return 0
	}
	return parseBytePair(value[:i], value[i:])
}

func parseBytePair(number, unit string) int64 {
	value, err := strconv.ParseFloat(strings.TrimSpace(number), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return -1
	}
	var multiplier float64
	switch strings.TrimSpace(unit) {
	case "B":
		multiplier = 1
	case "KiB":
		multiplier = 1 << 10
	case "MiB":
		multiplier = 1 << 20
	case "GiB":
		multiplier = 1 << 30
	case "TiB":
		multiplier = 1 << 40
	case "PiB":
		multiplier = 1 << 50
	default:
		return 0
	}
	scaled := value * multiplier
	if scaled >= float64(math.MaxInt64) {
		return -1
	}
	return int64(scaled)
}
