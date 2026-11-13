package externalvpn

import (
	"fmt"
	"maps"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
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
	_, err := m.checkpointTrafficStatsLocked(proc, nil)
	if err != nil {
		logger.Warning(fmt.Sprintf("%s inbound %d traffic handoff:", proc.protocol, proc.instance.ID), err)
	}
}

// Caller holds statsMu; strict boundaries require each changed user's current sample.
func (m *Manager) checkpointTrafficStatsLocked(proc *running, required []string) (bool, error) {
	previous := maps.Clone(proc.counters)
	rows, err := m.collectProcessTrafficMode(proc, len(required) > 0)
	if err != nil {
		proc.counters = previous
		return false, err
	}
	seen := map[string]bool{}
	samples := make([]sidecartraffic.Sample, 0, len(rows))
	for _, d := range rows {
		seen[d.Email] = true
		samples = append(samples, sidecartraffic.Sample{Email: d.Email, Up: d.Up, Down: d.Down})
	}
	for _, email := range required {
		if !seen[email] {
			proc.counters = previous
			return false, fmt.Errorf("external VPN: incomplete accounting identity boundary")
		}
	}
	accepted, err := proc.accounting.Store(samples)
	if !accepted {
		proc.counters = previous
	}
	return accepted, err
}

func (m *Manager) rebindTrafficOwnerLocked(proc *running, next *sidecartraffic.Owner) error {
	if proc.accounting.SameIdentities(next) {
		return nil
	}
	if next == nil {
		return fmt.Errorf("external VPN: current accounting owner unavailable")
	}
	proc.statsMu.Lock()
	defer proc.statsMu.Unlock()
	// These protocols expose no native per-client counter stream to split.
	if proc.protocol == model.TrustTunnel || proc.protocol == model.FPTN || proc.protocol == model.OpenFlux {
		accepted, err := m.checkpointTrafficStatsLocked(proc, proc.accounting.ChangedClients(next))
		if !accepted {
			return fmt.Errorf("external VPN: accounting identity handoff failed: %w", err)
		}
		if err != nil {
			logger.Warning(fmt.Sprintf("%s inbound %d identity handoff queued for retry:", proc.protocol, proc.instance.ID), err)
		}
	}
	proc.accounting = next
	proc.trafficEpoch++
	return nil
}
