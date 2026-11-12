package mieru

import (
	"fmt"
	"maps"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

func captureTrafficOwner(inst Instance) (*sidecartraffic.Owner, error) {
	emails := make([]string, 0, len(inst.Users))
	for _, user := range inst.Users {
		emails = append(emails, user.Name)
	}
	return sidecartraffic.Capture("mieru", inst.Id, emails, false)
}

// Called under m.mu, before changing users or deleting the old process/cursor.
func (m *Manager) handoffTrafficLocked(id int, cur *managed) {
	if cur.accounting == nil {
		return
	}
	if cur.proc == nil || !cur.proc.IsRunning() {
		logger.Warningf("mieru inbound %d: final counters unavailable after process exit", id)
		return
	}
	stats, err := scrapeUsersChecked(socketPathForID(id), configPathForID(id))
	if err != nil {
		logger.Warning(fmt.Sprintf("mieru inbound %d final traffic:", id), err)
		return
	}
	next := maps.Clone(m.traffic[id])
	if next == nil {
		next = map[string]trafficCursor{}
	}
	deltas, _ := collectInstanceTraffic(Instance{Id: id, Tag: cur.tag, Users: cur.users}, next, stats, time.Now())
	samples := make([]sidecartraffic.Sample, 0, len(deltas))
	for _, d := range deltas {
		samples = append(samples, sidecartraffic.Sample{Email: d.Email, Up: d.Up, Down: d.Down})
	}
	accepted, err := cur.accounting.Store(samples)
	if accepted {
		m.traffic[id] = next
		cur.trafficEpoch++
	}
	if err != nil {
		logger.Warning(fmt.Sprintf("mieru inbound %d traffic handoff queued for retry:", id), err)
	}
}
