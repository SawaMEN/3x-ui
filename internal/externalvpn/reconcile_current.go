package externalvpn

import (
	"context"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/sidecartraffic"
)

// ReconcileCurrent resolves the complete desired set while holding the same
// lock as startup/removal. Production callers must not replay a prefetched set.
func (m *Manager) ReconcileCurrent(parent context.Context, resolve func(context.Context) ([]Instance, error)) ([]Instance, error) {
	ctx, cancel := context.WithTimeout(parent, 5*time.Second)
	defer cancel()
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	desired, err := resolve(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return m.reconcileLocked(desired)
}

// RemoveIfInactive ignores a delayed delete when committed state still serves
// this manager. Shutdown and intentional forced removal use StopAll/Remove.
func (m *Manager) RemoveIfInactive(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	allowed, err := sidecartraffic.CanRemove(id, []model.Protocol{model.Pingtunnel, model.TrustTunnel, model.FPTN, model.VetroffProxy, model.OpenFlux}, func(row *model.Inbound, disabled map[string]bool) bool {
		inst, err := FromInbound(row)
		if err != nil {
			return false
		}
		if row.Protocol == model.Pingtunnel {
			return true
		}
		for _, client := range inst.Settings.Clients {
			if client.Enable && !disabled[client.Email] {
				return true
			}
		}
		return false
	})
	if err != nil {
		return err
	}
	if allowed {
		m.removeLocked(id)
	}
	return nil
}
