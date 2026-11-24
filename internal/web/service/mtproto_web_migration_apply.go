package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

type ManagedWebMigrationRequest struct {
	InboundID     int                         `json:"inboundId"`
	DraftRevision string                      `json:"draftRevision"`
	VPNMigrations []TelemtVPNMigrationRequest `json:"vpnMigrations"`
}

// Reuse the exact standalone workflow, but only from an explicitly saved
// disabled local draft. Never run network changes during an ordinary save.
func (s TelemtService) ApplyManagedWebMigration(ctx context.Context, req ManagedWebMigrationRequest, cert, key string) (TelemtWebProxyStatus, error) {
	telemtWebOperationMu.Lock()
	defer telemtWebOperationMu.Unlock()
	if req.InboundID <= 0 || req.DraftRevision == "" || len(req.VPNMigrations) == 0 {
		return TelemtWebProxyStatus{}, errors.New("managed WEB migration requires a saved draft and confirmed VPN moves")
	}
	var row model.Inbound
	if err := database.GetDB().First(&row, req.InboundID).Error; err != nil {
		return TelemtWebProxyStatus{}, err
	}
	if !mtprotoWebMigrationDraft(&row) || telemtVPNMigrationRevision(row) != req.DraftRevision {
		return TelemtWebProxyStatus{}, errors.New("MTProto draft changed; reopen the saved inbound")
	}
	opts, err := mtprotoWebForInbound(&row)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	active := row
	active.Settings, err = patchManagedMigrationDraft(row.Settings, opts.SharedIngress, false)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	active.Enable = true
	if err := validateInboundRuntimeTarget(&active); err != nil {
		return TelemtWebProxyStatus{}, err
	}
	if err := ValidateMtprotoWebInbound(&active); err != nil {
		return TelemtWebProxyStatus{}, err
	}
	if !mtprotoWebHasUsers(&active) {
		return TelemtWebProxyStatus{}, errors.New("managed MTProto WEB requires at least one active client")
	}
	options := TelemtWebProxyOptions{SharedIngress: opts.SharedIngress, Frontend: opts.Frontend, Carrier: opts.Carrier, Carriers: opts.Carriers, Site: opts.Site, BasePath: &opts.BasePath, Aliases: &opts.Aliases, RetryJitter: opts.RetryJitter, VPNMigrations: req.VPNMigrations}
	if options.Site == nil {
		options.Site = &TelemtWebSiteOptions{Mode: "auto"}
	}
	return s.enableWithVPNMigrationDraftLocked(ctx, opts.Domain, cert, key, options, &row)
}

func patchManagedMigrationDraft(settings string, ingress *TelemtWebIngress, draft bool) (string, error) {
	if ingress == nil || !ingress.Enabled {
		return "", errors.New("managed migration requires shared ingress")
	}
	var root, web map[string]json.RawMessage
	if err := json.Unmarshal([]byte(settings), &root); err != nil {
		return "", err
	}
	if err := json.Unmarshal(root["webProxy"], &web); err != nil || web == nil {
		return "", errors.New("invalid managed WEB migration settings")
	}
	var err error
	web["sharedIngress"], err = json.Marshal(ingress)
	if err != nil {
		return "", err
	}
	if draft {
		web["migrationDraft"] = json.RawMessage("true")
	} else {
		delete(web, "migrationDraft")
	}
	root["webProxy"], err = json.Marshal(web)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(root)
	return string(raw), err
}

func activateManagedMigrationDraftTx(tx *gorm.DB, draft *model.Inbound, ingress *TelemtWebIngress, journal *telemtVPNMigrationJournal) error {
	var current model.Inbound
	if err := tx.First(&current, draft.Id).Error; err != nil {
		return err
	}
	if !mtprotoWebMigrationDraft(&current) || telemtVPNMigrationRevision(current) != telemtVPNMigrationRevision(*draft) {
		return errors.New("MTProto draft changed; refresh migration preview")
	}
	opts, err := mtprotoWebForInbound(&current)
	if err != nil {
		return err
	}
	current.Settings, err = patchManagedMigrationDraft(current.Settings, ingress, false)
	if err != nil {
		return err
	}
	current.Enable = true
	current.Listen, current.Port = "", 443
	conflict, err := checkPortConflictTx(tx, &current, current.Id)
	if err != nil {
		return err
	}
	if conflict != nil {
		return errors.New(conflict.String())
	}
	// Check WEB uniqueness inside the same transaction as activation.
	var rows []model.Inbound
	if err := tx.Where("node_id IS NULL AND protocol = ? AND enable = ? AND id <> ?", model.MTProto, true, current.Id).Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		other, err := mtprotoWebForInbound(&row)
		if err != nil {
			return err
		}
		if other.Enabled {
			return fmt.Errorf("MTProto WEB already belongs to inbound %d", row.Id)
		}
	}
	journal.ManagedInboundID, journal.ManagedOriginalIngress = current.Id, opts.SharedIngress
	return tx.Model(&model.Inbound{}).Where("id = ?", current.Id).Updates(map[string]any{"enable": true, "listen": "", "port": 443, "settings": current.Settings}).Error
}

func restoreManagedMigrationDraftTx(tx *gorm.DB, journal *telemtVPNMigrationJournal) error {
	if journal.ManagedInboundID == 0 {
		return nil
	}
	var row model.Inbound
	if err := tx.First(&row, journal.ManagedInboundID).Error; err != nil {
		if database.IsNotFound(err) {
			return nil
		}
		return err
	}
	if row.Protocol != model.MTProto || row.NodeID != nil {
		return errors.New("managed migration owner changed; refusing unsafe rollback")
	}
	opts, err := mtprotoWebForInbound(&row)
	if err != nil {
		return err
	}
	// A previous recovery may already have restored the draft. Do not
	// overwrite subsequent edits when retrying runtime restoration.
	if mtprotoWebMigrationDraft(&row) {
		return nil
	}
	if !opts.Enabled || row.Listen != "" || row.Port != 443 {
		return errors.New("managed WEB settings changed; refusing unsafe rollback")
	}
	settings, err := patchManagedMigrationDraft(row.Settings, journal.ManagedOriginalIngress, true)
	if err != nil {
		return err
	}
	return tx.Model(&model.Inbound{}).Where("id = ?", row.Id).Updates(map[string]any{"enable": false, "settings": settings, "listen": "", "port": 443}).Error
}
