package mtproto

import (
	"fmt"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

func captureTrafficOwner(inst Instance) (*sidecartraffic.Owner, error) {
	emails := make([]string, 0, len(inst.Secrets))
	for _, secret := range inst.Secrets {
		emails = append(emails, secret.Name)
	}
	return sidecartraffic.Capture("mtproto", inst.Id, emails, inst.RouteThroughXray)
}

// Lifecycle stop/reload remains best effort; identity rebind requires success.
func (m *Manager) handoffTrafficLocked(id int, cur *managed) {
	if cur.accounting == nil {
		return
	}
	_, err := m.checkpointTrafficLocked(id, cur, nil)
	if err != nil {
		logger.Warning(fmt.Sprintf("mtproto inbound %d traffic handoff:", id), err)
	}
}

func (m *Manager) checkpointTrafficLocked(id int, cur *managed, required []string) (bool, error) {
	if cur.proc == nil || !cur.proc.IsRunning() {
		return false, fmt.Errorf("mtproto: final counters unavailable after process exit")
	}
	users, err := scrapeStatsChecked(cur.apiPort, cur.apiToken)
	if err != nil {
		return false, err
	}
	for _, email := range required {
		if _, ok := users[nativeUsername(email)]; !ok {
			return false, fmt.Errorf("mtproto: incomplete accounting identity boundary")
		}
	}
	next, deltas, _ := mergeCounterSnapshots(cur.last, users)
	samples := make([]sidecartraffic.Sample, 0, len(deltas))
	for user, d := range deltas {
		samples = append(samples, sidecartraffic.Sample{Email: panelUsername(user, cur.userEmails), Up: d.up, Down: d.down})
	}
	accepted, err := cur.accounting.Store(samples)
	if accepted {
		cur.last = next
		cur.trafficEpoch++
	}
	return accepted, err
}

func (m *Manager) rebindTrafficOwnerLocked(id int, cur *managed, next *sidecartraffic.Owner) error {
	if cur.accounting.SameIdentities(next) {
		return nil
	}
	if next == nil {
		return fmt.Errorf("mtproto: current accounting owner unavailable")
	}
	accepted, err := m.checkpointTrafficLocked(id, cur, cur.accounting.ChangedClients(next))
	if !accepted {
		return fmt.Errorf("mtproto: accounting identity handoff failed: %w", err)
	}
	if err != nil {
		logger.Warning(fmt.Sprintf("mtproto inbound %d identity handoff queued for retry:", id), err)
	}
	cur.accounting = next
	return nil
}

func (m *Manager) stopWithTrafficLocked(id int, cur *managed) error {
	m.handoffTrafficLocked(id, cur)
	return stopManaged(cur)
}
