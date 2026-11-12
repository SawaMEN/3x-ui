package sidecartraffic

import (
	"context"
	"errors"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// ErrRevoked means a saved desired instance no longer represents an enabled
// local inbound. Accounting ownership alone is never startup authorization.
var ErrRevoked = errors.New("sidecar inbound is no longer enabled and local with the requested protocol")

type Desired struct {
	ID       int
	Protocol model.Protocol
}

// Current returns one decision per desired instance. Callers hold their manager
// lock until applying these decisions, so a subsequent CRUD Remove joins any
// startup authorized before its database change. Query failures must leave the
// existing runtime untouched; they are different from authoritative revocation.
func Current(desired []Desired) ([]bool, error) {
	allowed := make([]bool, len(desired))
	db := database.GetDB()
	if db == nil {
		// Standalone managers have no panel database as their source of truth.
		for i := range allowed {
			allowed[i] = true
		}
		return allowed, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ids := make([]int, len(desired))
	for i, inst := range desired {
		ids[i] = inst.ID
	}
	current := make(map[int]model.Protocol, len(ids))
	for start := 0; start < len(ids); start += 900 {
		var rows []model.Inbound
		if err := db.WithContext(ctx).Select("id", "enable", "node_id", "protocol").Where("id IN ?", ids[start:min(start+900, len(ids))]).Find(&rows).Error; err != nil {
			return nil, errors.New("read current sidecar inbound state failed")
		}
		for _, row := range rows {
			if row.Enable && row.NodeID == nil {
				current[row.Id] = row.Protocol
			}
		}
	}
	for i, inst := range desired {
		protocol, ok := current[inst.ID]
		allowed[i] = ok && protocol == inst.Protocol
	}
	return allowed, nil
}

func ValidateDesired(id int, protocol model.Protocol) error {
	allowed, err := Current([]Desired{{ID: id, Protocol: protocol}})
	if err != nil {
		return err
	}
	if !allowed[0] {
		return ErrRevoked
	}
	return nil
}
