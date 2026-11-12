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

func (m *Manager) handoffTrafficLocked(id int, cur *managed) {
	if cur.accounting == nil {
		return
	}
	if cur.proc == nil || !cur.proc.IsRunning() {
		logger.Warningf("mtproto inbound %d: final counters unavailable after process exit", id)
		return
	}
	users, err := scrapeStatsChecked(cur.apiPort, cur.apiToken)
	if err != nil {
		logger.Warning(fmt.Sprintf("mtproto inbound %d final traffic:", id), err)
		return
	}
	next, deltas, _ := mergeCounterSnapshots(cur.last, users)
	samples := make([]sidecartraffic.Sample, 0, len(deltas))
	for name, d := range deltas {
		samples = append(samples, sidecartraffic.Sample{Email: panelUsername(name, cur.userEmails), Up: d.up, Down: d.down})
	}
	accepted, err := cur.accounting.Store(samples)
	if accepted {
		cur.last = next
		cur.trafficEpoch++
	}
	if err != nil {
		logger.Warning(fmt.Sprintf("mtproto inbound %d traffic handoff queued for retry:", id), err)
	}
}

func (m *Manager) stopWithTrafficLocked(id int, cur *managed) error {
	m.handoffTrafficLocked(id, cur)
	return stopManaged(cur)
}
