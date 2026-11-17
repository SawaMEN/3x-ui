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

// Lifecycle stop/reload remains best effort; identity rebind requires success.
func (m *Manager) handoffTrafficLocked(id int, cur *managed) {
	if cur.accounting == nil {
		return
	}
	_, err := m.checkpointTrafficLocked(id, cur, nil)
	if err != nil {
		logger.Warning(fmt.Sprintf("mieru inbound %d traffic handoff:", id), err)
	}
}

func (m *Manager) checkpointTrafficLocked(id int, cur *managed, required []string) (bool, error) {
	if cur.proc == nil || !cur.proc.IsRunning() {
		return false, fmt.Errorf("mieru: final counters unavailable after process exit")
	}
	stats, err := scrapeUsersChecked(socketPathForID(id), configPathForID(id))
	if err != nil {
		return false, err
	}
	for _, email := range required {
		if _, ok := stats[email]; !ok {
			return false, fmt.Errorf("mieru: incomplete accounting identity boundary")
		}
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
	return accepted, err
}

func (m *Manager) rebindTrafficOwnerLocked(id int, cur *managed, next *sidecartraffic.Owner) error {
	if cur.accounting.SameIdentities(next) {
		return nil
	}
	if next == nil {
		return fmt.Errorf("mieru: current accounting owner unavailable")
	}
	accepted, err := m.checkpointTrafficLocked(id, cur, cur.accounting.ChangedClients(next))
	if !accepted {
		return fmt.Errorf("mieru: accounting identity handoff failed: %w", err)
	}
	if err != nil {
		logger.Warning(fmt.Sprintf("mieru inbound %d identity handoff queued for retry:", id), err)
	}
	cur.accounting = next
	return nil
}
