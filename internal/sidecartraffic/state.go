package sidecartraffic

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// ErrRevoked means a saved desired instance no longer represents an enabled
// local inbound. Accounting ownership alone is never startup authorization.
var ErrRevoked = errors.New("sidecar inbound is no longer enabled and local with the requested protocol")

// ErrStaleDesired rejects an obsolete or unversioned panel payload before any
// runtime mutation. Reconcile must preserve the newer running configuration.
var ErrStaleDesired = errors.New("sidecar desired configuration is stale; rebuild from current panel state")

type Desired struct {
	Revision       string
	ClientRevision string
	Clients        []string
	ID             int
	Protocol       model.Protocol
}

// Current returns one decision per desired instance. Callers hold their manager
// lock until applying these decisions, so a subsequent CRUD Remove joins any
// startup authorized before its database change. Query failures must leave the
// existing runtime untouched; they are different from authoritative revocation.
func Current(desired []Desired) ([]bool, error) {
	allowed := make([]bool, len(desired))
	if len(desired) == 0 {
		return allowed, nil
	}
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
	current, disabled, err := readState(db.WithContext(ctx), ids)
	if err != nil {
		return nil, errors.New("read current sidecar inbound state failed")
	}
	for i, inst := range desired {
		row, ok := current[inst.ID]
		allowed[i] = ok && row.Enable && row.NodeID == nil && row.Protocol == inst.Protocol
		if !allowed[i] {
			continue
		}
		if inst.Revision == "" || inst.Revision != RevisionOf(&row) || inst.ClientRevision != DisabledRevision(disabled[inst.ID]) {
			return nil, ErrStaleDesired
		}
		for _, email := range inst.Clients {
			if disabled[inst.ID][email] {
				return nil, ErrStaleDesired
			}
		}
	}
	return allowed, nil
}

func ValidateDesired(desired Desired) error {
	allowed, err := Current([]Desired{desired})
	if err != nil {
		return err
	}
	if !allowed[0] {
		return ErrRevoked
	}
	return nil
}

func readState(db *gorm.DB, ids []int) (map[int]model.Inbound, map[int]map[string]bool, error) {
	current := make(map[int]model.Inbound, len(ids))
	disabled := make(map[int]map[string]bool)
	err := db.Transaction(func(tx *gorm.DB) error {
		for start := 0; start < len(ids); start += 900 {
			batch := ids[start:min(start+900, len(ids))]
			var rows []model.Inbound
			if err := tx.Select("id", "enable", "node_id", "protocol", "tag", "listen", "port", "settings").Where("id IN ?", batch).Find(&rows).Error; err != nil {
				return err
			}
			for _, row := range rows {
				current[row.Id] = row
			}
			var stats []coremodel.ClientTraffic
			if err := tx.Select("inbound_id", "email").Where("inbound_id IN ? AND enable = ?", batch, false).Find(&stats).Error; err != nil {
				return err
			}
			for _, row := range stats {
				if disabled[row.InboundId] == nil {
					disabled[row.InboundId] = make(map[string]bool)
				}
				disabled[row.InboundId][row.Email] = true
			}
		}
		return nil
	})
	return current, disabled, err
}
