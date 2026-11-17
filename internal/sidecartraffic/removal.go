package sidecartraffic

import (
	"context"
	"errors"
	"slices"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// CanRemove checks committed state, not the delayed delete payload. Callers hold
// their manager mutex across this decision and the removal. Force shutdown and
// explicit low-level Remove remain separate operations.
func CanRemove(id int, protocols []model.Protocol, active func(*model.Inbound, map[string]bool) bool) (bool, error) {
	db := database.GetDB()
	if db == nil {
		return true, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rows, disabled, err := readState(db.WithContext(ctx), []int{id})
	if err != nil {
		return false, errors.New("read current sidecar removal state failed")
	}
	row, ok := rows[id]
	if !ok || !row.Enable || row.NodeID != nil || !slices.Contains(protocols, row.Protocol) {
		return true, nil
	}
	return !active(&row, disabled[id]), nil
}
