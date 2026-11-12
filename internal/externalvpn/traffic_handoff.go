package externalvpn

import (
	"fmt"
	"maps"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

func captureTrafficOwner(inst Instance) (*sidecartraffic.Owner, error) {
	emails := make([]string, 0, len(inst.Settings.Clients))
	for _, client := range inst.Settings.Clients {
		if client.Enable {
			emails = append(emails, client.Email)
		}
	}
	return sidecartraffic.Capture("externalvpn", inst.ID, emails, false)
}

func (m *Manager) handoffTrafficLocked(proc *running) {
	if proc.accounting == nil {
		return
	}
	proc.statsMu.Lock()
	defer proc.statsMu.Unlock()
	old := maps.Clone(proc.counters)
	rows, err := m.collectProcessTrafficChecked(proc)
	if err != nil {
		logger.Warning(fmt.Sprintf("%s inbound %d final traffic:", proc.protocol, proc.instance.ID), err)
		return
	}
	samples := make([]sidecartraffic.Sample, 0, len(rows))
	for _, d := range rows {
		samples = append(samples, sidecartraffic.Sample{Email: d.Email, Up: d.Up, Down: d.Down})
	}
	accepted, err := proc.accounting.Store(samples)
	if !accepted {
		proc.counters = old
	}
	if err != nil {
		logger.Warning(fmt.Sprintf("%s inbound %d traffic handoff queued for retry:", proc.protocol, proc.instance.ID), err)
	}
}
