package service

import (
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// SidecarTrafficOwners is captured before consuming runtime counters. Names are
// only lookup keys at capture time; a retained batch is charged by row identity.
type SidecarTrafficOwners struct {
	Inbounds map[string]int
	Clients  map[string]int
}

type SidecarTrafficDelta struct {
	ID   int   `json:"id"`
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

type SidecarTrafficBatch struct {
	ID       string                `json:"id"`
	Inbounds []SidecarTrafficDelta `json:"inbounds"`
	Clients  []SidecarTrafficDelta `json:"clients"`
}

func CaptureSidecarTrafficOwners(protocols []model.Protocol) (SidecarTrafficOwners, error) {
	owners := SidecarTrafficOwners{Inbounds: map[string]int{}, Clients: map[string]int{}}
	err := database.GetDB().Transaction(func(tx *gorm.DB) error {
		var inbounds []model.Inbound
		if err := tx.Select("id", "tag").Where("protocol IN ? AND node_id IS NULL", protocols).Find(&inbounds).Error; err != nil {
			return err
		}
		ids := make([]int, 0, len(inbounds))
		for _, row := range inbounds {
			owners.Inbounds[row.Tag] = row.Id
			ids = append(ids, row.Id)
		}
		for _, page := range chunkInts(ids, sqlInChunk) {
			var clients []coremodel.ClientTraffic
			// Include legacy traffic owners and normalized shared-client links. Disabled
			// rows still own the final sample drained when reconciliation removes them.
			linked := tx.Table("client_inbounds ci").Select("c.email").Joins("JOIN clients c ON c.id = ci.client_id").Where("ci.inbound_id IN ?", page)
			if err := tx.Select("id", "email").Where("inbound_id IN ? OR email IN (?)", page, linked).Find(&clients).Error; err != nil {
				return err
			}
			for _, row := range clients {
				owners.Clients[row.Email] = row.Id
			}
		}
		return nil
	})
	return owners, err
}

func (o SidecarTrafficOwners) Batch(id string, inbounds []*coremodel.Traffic, clients []*coremodel.ClientTraffic) SidecarTrafficBatch {
	batch := SidecarTrafficBatch{ID: id}
	for _, t := range inbounds {
		if t != nil && t.IsInbound && o.Inbounds[t.Tag] > 0 {
			batch.Inbounds = append(batch.Inbounds, SidecarTrafficDelta{ID: o.Inbounds[t.Tag], Up: t.Up, Down: t.Down})
		}
	}
	for _, t := range clients {
		if t != nil && o.Clients[t.Email] > 0 {
			batch.Clients = append(batch.Clients, SidecarTrafficDelta{ID: o.Clients[t.Email], Up: t.Up, Down: t.Down})
		}
	}
	return batch
}

// AddSidecarTrafficBatch commits the receipt with the bytes. Post-commit quota
// maintenance follows the normal traffic path and must never cause replay.
func (s *InboundService) AddSidecarTrafficBatch(batch SidecarTrafficBatch) (needRestart, disabled bool, err error) {
	if batch.ID == "" {
		return false, false, errors.New("sidecar traffic batch has no ID")
	}
	var nodes []int
	var plans []trafficInboundUpdatePlan
	err = submitTrafficWrite(func() error {
		applied := false
		if err := database.GetDB().Transaction(func(tx *gorm.DB) error {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&model.SidecarTrafficReceipt{ID: batch.ID})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil
			}
			applied = true
			for _, t := range batch.Inbounds {
				if t.ID <= 0 {
					return errors.New("invalid sidecar inbound identity")
				}
				if err := tx.Model(&model.Inbound{}).Where("id = ? AND node_id IS NULL", t.ID).Updates(map[string]any{"up": gorm.Expr(database.ClampedAddExpr("up"), t.Up), "down": gorm.Expr(database.ClampedAddExpr("down"), t.Down)}).Error; err != nil {
					return err
				}
			}
			clients := make([]*coremodel.ClientTraffic, 0, len(batch.Clients))
			for _, t := range batch.Clients {
				if t.ID <= 0 {
					return errors.New("invalid sidecar client identity")
				}
				clients = append(clients, &coremodel.ClientTraffic{TuicTrafficID: t.ID, Up: t.Up, Down: t.Down})
			}
			return s.addClientTraffic(tx, clients)
		}); err != nil {
			return err
		}
		if applied {
			needRestart, disabled, nodes, plans, _ = s.maintainTrafficLocked()
		}
		return nil
	})
	if err == nil {
		needRestart = s.applyTrafficRemotePlans(plans) || needRestart
		if len(nodes) > 0 {
			s.restartRemoteNodesOnDisable(nodes)
		}
	}
	return
}
