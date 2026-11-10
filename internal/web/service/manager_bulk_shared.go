package service

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

// ManagerBulkSharedInput creates independent subscriptions and client accounts
// against already installed, shared inbounds. Unlike generated subscriptions,
// the selected listeners never become manager-owned and are not cloned/deleted
// by a subscription lifecycle operation.
type ManagerBulkSharedInput struct {
	NamePrefix string `json:"namePrefix"`
	Count      int    `json:"count"`
	InboundIDs []int  `json:"inboundIds"`
}

type ManagerBulkSharedResult struct {
	Created int   `json:"created"`
	Queued  int   `json:"queued"`
	IDs     []int `json:"ids"`
}

const maxManagerBulkClients = 100

// validateSharedInboundPool requires real, client-attachable inbounds owned by
// the caller. Manager-generated listeners cannot be used as shared templates:
// their lifecycle still belongs to an existing subscription and its cleanup.
func validateSharedInboundPool(owner int, inboundIDs []int) ([]model.ManagerEntry, error) {
	if len(inboundIDs) == 0 || len(inboundIDs) > 100 {
		return nil, fmt.Errorf("select between 1 and 100 shared inbounds")
	}
	seen := make(map[int]bool, len(inboundIDs))
	for _, id := range inboundIDs {
		if id < 1 || seen[id] {
			return nil, fmt.Errorf("inbound IDs must be unique and positive")
		}
		seen[id] = true
	}
	var inbounds []model.Inbound
	if err := database.GetDB().Where("user_id = ? AND id IN ?", owner, inboundIDs).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	if len(inbounds) != len(inboundIDs) {
		return nil, fmt.Errorf("one or more inbounds do not exist or belong to another owner")
	}
	byID := make(map[int]model.Inbound, len(inbounds))
	for _, ib := range inbounds {
		byID[ib.Id] = ib
	}
	entries := make([]model.ManagerEntry, 0, len(inboundIDs))
	for _, id := range inboundIDs {
		ib := byID[id]
		if !ib.Enable {
			return nil, fmt.Errorf("shared inbound %d is disabled", id)
		}
		if !model.InboundProtocolCapabilities[ib.Protocol].ClientAttachable {
			return nil, fmt.Errorf("shared inbound %d does not support individual clients", id)
		}
		if strings.Contains(ib.Tag, "manager-") {
			return nil, fmt.Errorf("shared inbound %d is owned by another manager generation", id)
		}
		entries = append(entries, model.ManagerEntry{
			TemplateID: id, Mode: "existing", Label: ib.Remark, Port: strconv.Itoa(ib.Port),
		})
	}
	return entries, nil
}

// BulkCreateShared is atomic: either all subscription rows and their queued
// jobs exist, or nothing is saved. Jobs use the already tested manager worker to
// create one independent client identity and attach it to the chosen inbounds.
// The jobs can subsequently fail individually; their status remains visible in
// the manager operations journal and can be retried.
func (s *ManagerService) BulkCreateShared(owner int, req ManagerBulkSharedInput) (ManagerBulkSharedResult, error) {
	result := ManagerBulkSharedResult{IDs: []int{}}
	// Reject line breaks before trimming. Otherwise a crafted leading newline
	// is silently removed and the same malformed request is accepted.
	if strings.ContainsAny(req.NamePrefix, "\r\n") {
		return result, fmt.Errorf("client name must not contain line breaks")
	}
	name := strings.TrimSpace(req.NamePrefix)
	if owner <= 0 || req.Count < 1 || req.Count > maxManagerBulkClients ||
		name == "" || len(name) > 220 {
		return result, fmt.Errorf("provide a name and between 1 and 100 clients")
	}
	managerMu.Lock()
	defer managerMu.Unlock()

	entries, err := validateSharedInboundPool(owner, req.InboundIDs)
	if err != nil {
		return result, err
	}
	// Validate the exact shape through the existing manager validation.
	prototype := model.ManagerSubscription{
		UserID: owner, Name: name + " 001", Enabled: true,
		IntervalMinutes: 60, GraceMinutes: 120, Entries: entries,
	}
	if err := s.validate(&prototype); err != nil {
		return result, err
	}
	ids := make([]int, 0, req.Count)
	now := time.Now().UnixMilli()
	err = database.GetDB().Transaction(func(tx *gorm.DB) error {
		for i := 0; i < req.Count; i++ {
			sub := model.ManagerSubscription{
				UserID: owner, Name: fmt.Sprintf("%s %03d", name, i+1),
				Token: uuid.NewString(), Enabled: true,
				IntervalMinutes: 60, GraceMinutes: 120, NextRotation: now + int64(time.Hour/time.Millisecond),
				Entries: append([]model.ManagerEntry(nil), entries...),
			}
			if err := tx.Create(&sub).Error; err != nil {
				return err
			}
			key := fmt.Sprintf("subscription/%d", sub.ID)
			op := model.ManagerOperation{
				UserID: owner, SubscriptionID: sub.ID, Kind: "rotate",
				State: "queued", ActiveKey: &key, Total: len(entries),
			}
			if err := tx.Create(&op).Error; err != nil {
				return err
			}
			ids = append(ids, sub.ID)
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	return ManagerBulkSharedResult{Created: len(ids), Queued: len(ids), IDs: ids}, nil
}
