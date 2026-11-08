package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

var errManagerCleanupPending = errors.New("cleanup pending")

func managerKeep(sub *model.ManagerSubscription) map[int]bool {
	keep := map[int]bool{}
	for _, e := range sub.Published {
		keep[e.ActiveInboundID] = true
	}
	for _, generation := range sub.Retired {
		if generation.ExpiresAt > time.Now().UnixMilli() {
			for _, e := range generation.Entries {
				keep[e.ActiveInboundID] = true
			}
		}
	}
	return keep
}

func (s *ManagerService) rollbackGeneration(ctx context.Context, op *model.ManagerOperation, sub *model.ManagerSubscription) error {
	var owned []model.Inbound
	if err := database.GetDB().Where("user_id = ? AND tag LIKE ?", sub.UserID, "%manager-"+sub.Token+"-%").Find(&owned).Error; err != nil {
		return err
	}
	var persisted model.ManagerSubscription
	if err := database.GetDB().First(&persisted, sub.ID).Error; err != nil {
		return err
	}
	keep := managerKeep(&persisted)
	prefix := fmt.Sprintf("manager-%s-%d-", sub.Token, op.ID)
	for _, ib := range owned {
		tag := strings.TrimPrefix(ib.Tag, nodeTagPrefix(ib.NodeID))
		if !strings.HasPrefix(tag, prefix) {
			keep[ib.Id] = true
		}
	}
	return s.cleanup(ctx, op, sub, keep)
}

func (s *ManagerService) cleanupGeneration(ctx context.Context, op *model.ManagerOperation, sub *model.ManagerSubscription) error {
	relays := map[int]bool{}
	for _, e := range sub.Published {
		if e.RelayID > 0 {
			relays[e.RelayID] = true
		}
	}
	for _, g := range sub.Retired {
		for _, e := range g.Entries {
			if e.RelayID > 0 {
				relays[e.RelayID] = true
			}
		}
	}
	if err := s.cleanup(ctx, op, sub, managerKeep(sub)); err != nil {
		return err
	}
	retired := sub.Retired[:0]
	for _, generation := range sub.Retired {
		if generation.ExpiresAt > time.Now().UnixMilli() {
			retired = append(retired, generation)
		}
	}
	sub.Retired = retired
	raw, err := json.Marshal(retired)
	if err != nil {
		return err
	}
	if err := database.GetDB().Model(sub).Update("retired", string(raw)).Error; err != nil {
		return err
	}
	for id := range relays {
		var relay model.ManagerRelay
		if err := database.GetDB().First(&relay, id).Error; err != nil {
			return err
		}
		if relay.Installed {
			if err := s.applyRelay(ctx, relay, true, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *ManagerService) scheduleMaintenance() {
	db := database.GetDB()
	var subs []model.ManagerSubscription
	if err := db.Select("id", "user_id", "retired").Find(&subs).Error; err != nil {
		return
	}
	for _, sub := range subs {
		due := false
		for _, g := range sub.Retired {
			if g.ExpiresAt <= time.Now().UnixMilli() {
				due = true
				break
			}
		}
		if !due {
			var count int64
			if err := db.Model(&model.ManagerOperation{}).Where("subscription_id = ? AND state IN ?", sub.ID, []string{"cleanup_pending", "interrupted"}).Count(&count).Error; err != nil {
				continue
			}
			due = count > 0
		}
		if due {
			if _, err := s.enqueue(sub.UserID, sub.ID, "cleanup"); err != nil {
				continue
			}
		}
	}
}
