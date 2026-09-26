package externalvpn

import "github.com/SawaMEN/3x-ui/v3/internal/database/model"

// StopProtocol stops only processes that use the selected external VPN binary.
// Enabled inbounds are started again by the regular reconciliation job.
func (m *Manager) StopProtocol(protocol model.Protocol) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, proc := range m.procs {
		if proc.protocol == protocol {
			m.removeLocked(id)
		}
	}
}
