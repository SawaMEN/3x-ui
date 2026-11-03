package service

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

func (s *ManagerService) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	db := database.GetDB()
	if err := db.Model(&model.ManagerOperation{}).Where("state = ?", "running").Updates(map[string]any{"state": "interrupted", "active_key": nil, "message": "Panel restarted; inspect before retrying", "finished_at": time.Now().UnixMilli()}).Error; err != nil {
		logger.Error("manager recovery:", err)
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var next time.Time
	workers := make(chan struct{}, 4)
	var jobs sync.WaitGroup
	defer jobs.Wait()
	for {
		if ctx.Err() != nil {
			return
		}
		if time.Now().After(next) {
			s.schedule()
			next = time.Now().Add(time.Minute)
		}
		var op model.ManagerOperation
		if err := db.Where("state = ?", "queued").Order("id").First(&op).Error; err == nil {
			select {
			case workers <- struct{}{}:
				jobs.Add(1)
				go func(op model.ManagerOperation) { defer jobs.Done(); defer func() { <-workers }(); s.execute(ctx, &op) }(op)
			default:
			}
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			logger.Error("manager queue:", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *ManagerService) schedule() {
	managerMu.Lock()
	defer managerMu.Unlock()
	db := database.GetDB()
	var subs []model.ManagerSubscription
	if err := db.Where("enabled = ? AND auto_rotate = ? AND next_rotation <= ?", true, true, time.Now().UnixMilli()).Find(&subs).Error; err != nil {
		logger.Error("manager schedule:", err)
		return
	}
	for _, sub := range subs {
		if _, err := s.enqueue(sub.UserID, sub.ID, "rotate"); err != nil {
			logger.Error("manager enqueue:", err)
			continue
		}
		if err := db.Model(&sub).Update("next_rotation", time.Now().Add(time.Duration(sub.IntervalMinutes)*time.Minute).UnixMilli()).Error; err != nil {
			logger.Error("manager schedule:", err)
		}
	}
	s.scheduleMaintenance()
	if err := db.Where("finished_at > 0 AND finished_at < ?", time.Now().Add(-30*24*time.Hour).UnixMilli()).Delete(&model.ManagerOperation{}).Error; err != nil {
		logger.Error("manager journal:", err)
	}
	var excess []int
	if err := db.Model(&model.ManagerOperation{}).Where("state NOT IN ?", []string{"queued", "running"}).Order("id DESC").Offset(50000).Limit(1000).Pluck("id", &excess).Error; err == nil && len(excess) > 0 {
		if err := db.Delete(&model.ManagerOperation{}, excess).Error; err != nil {
			logger.Error("manager journal:", err)
		}
	}
}

func (s *ManagerService) execute(ctx context.Context, op *model.ManagerOperation) {
	managerMu.Lock()
	db := database.GetDB()
	q := db.Model(op).Where("state = ?", "queued").Update("state", "running")
	managerMu.Unlock()
	if q.Error != nil || q.RowsAffected != 1 {
		return
	}
	started := time.Now()
	var workErr error
	defer func() {
		if v := recover(); v != nil {
			workErr = fmt.Errorf("operation panic: %v", v)
		}
		op.State, op.Message = "success", "Completed"
		if workErr != nil {
			op.State, op.Message = "failed", workErr.Error()
			if errors.Is(workErr, errManagerCleanupPending) {
				op.State = "cleanup_pending"
			}
		}
		if ctx.Err() != nil {
			op.State, op.Message = "interrupted", "Panel stopped; inspect before retrying"
		}
		logger.Infof("component=manager operation_id=%d kind=%s state=%s completed=%d total=%d elapsed_ms=%d", op.ID, op.Kind, op.State, op.Completed, op.Total, time.Since(started).Milliseconds())
		op.ActiveKey = nil
		op.FinishedAt = time.Now().UnixMilli()
		if err := db.Save(op).Error; err != nil {
			logger.Error("manager result:", err)
		}
	}()
	if op.RelayID > 0 {
		release := managerLockKeys([]string{fmt.Sprintf("relay/%d", op.RelayID)})
		defer release()
		workErr = s.executeRelay(ctx, op)
		return
	}
	var sub model.ManagerSubscription
	if workErr = db.Where("id = ? AND user_id = ?", op.SubscriptionID, op.UserID).First(&sub).Error; workErr != nil {
		return
	}
	release := managerLockSubscription(&sub)
	defer release()
	if op.Kind == "cleanup" {
		workErr = s.cleanupGeneration(ctx, op, &sub)
		if workErr == nil {
			op.Completed = op.Total
			workErr = db.Model(&model.ManagerOperation{}).Where("subscription_id = ? AND id <> ? AND state IN ?", sub.ID, op.ID, []string{"cleanup_pending", "interrupted"}).Update("state", "recovered").Error
		}
		if workErr != nil {
			workErr = fmt.Errorf("%w: %w", errManagerCleanupPending, workErr)
		}
		return
	}
	if op.Kind == "delete" {
		workErr = s.deleteSubscription(ctx, op, &sub)
		return
	}
	if !sub.Enabled {
		workErr = fmt.Errorf("subscription is disabled")
		return
	}
	workErr = s.rotate(ctx, op, &sub)
}

func managerMarkRestart(needed bool) {
	if !needed {
		return
	}
	kind, err := (&SettingService{}).GetCoreType()
	if err == nil && IsNativeCore(kind) {
		NativeCore(kind).SetToNeedRestart()
	} else {
		(&CoreService{}).SetToNeedRestart()
	}
}

func managerRetry(ctx context.Context, op *model.ManagerOperation, fn func() error) error {
	var err error
	for i := 1; i <= 3; i++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		op.Attempts++
		if e := database.GetDB().Model(op).Update("attempts", op.Attempts).Error; e != nil {
			return e
		}
		if err = fn(); err == nil {
			return nil
		}
		op.Message = fmt.Sprintf("Attempt %d/3: %v", i, err)
		if e := database.GetDB().Model(op).Update("message", op.Message).Error; e != nil {
			return e
		}
		if i < 3 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(i) * time.Second):
			}
		}
	}
	return err
}

func (s *ManagerService) rotate(ctx context.Context, op *model.ManagerOperation, sub *model.ManagerSubscription) (result error) {
	report := s.Preflight(ctx, sub.UserID, *sub)
	if !report.Ready {
		for _, entry := range report.Entries {
			if !entry.Ready {
				return fmt.Errorf("entry %d preflight: %s", entry.Index, entry.Message)
			}
		}
	}
	published := false
	defer func() {
		if v := recover(); v != nil {
			result = fmt.Errorf("generation panic: %v", v)
		}
		if published {
			return
		}
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		if err := s.rollbackGeneration(rollbackCtx, op, sub); err != nil {
			result = fmt.Errorf("%w: %w; rollback: %w", errManagerCleanupPending, result, err)
		}
	}()
	if err := s.cleanupGeneration(ctx, op, sub); err != nil {
		return fmt.Errorf("%w: %w", errManagerCleanupPending, err)
	}
	finish := beginLocalConfigBatch()
	defer finish()
	db := database.GetDB()
	ibs := &InboundService{}
	clients := &ClientService{}
	entries := slices.Clone(sub.Entries)
	for i := range entries {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := managerStep(op, fmt.Sprintf("Preparing connection %d/%d", i+1, len(entries))); err != nil {
			return err
		}
		e := &entries[i]
		if e.Mode == "custom" {
			op.Completed = i + 1
			continue
		}
		var ib *model.Inbound
		var err error
		if e.Mode == "existing" {
			ib, err = ibs.GetInbound(e.TemplateID)
		} else {
			ib, err = s.clone(ctx, op, sub, *e, i)
		}
		if err != nil {
			return fmt.Errorf("entry %d: %w", i+1, err)
		}
		if ib.UserId != sub.UserID {
			return fmt.Errorf("inbound owner mismatch")
		}
		if sub.ClientID == 0 {
			payload := &ClientCreatePayload{Client: model.Client{Email: "manager-" + sub.Token, SubID: sub.Token, Enable: true}, InboundIds: []int{ib.Id}}
			needed, createErr := clients.Create(ibs, payload)
			managerMarkRestart(needed)
			var rec model.ClientRecord
			lookup := db.Where("email = ?", payload.Client.Email).First(&rec).Error
			if lookup == nil {
				sub.ClientID, sub.OwnClient = rec.Id, true
				if err := db.Model(sub).Updates(map[string]any{"client_id": rec.Id, "own_client": true}).Error; err != nil {
					return err
				}
			}
			if createErr != nil {
				return createErr
			}
			if lookup != nil {
				return lookup
			}
		}
		if err := managerRetry(ctx, op, func() error {
			needed, err := clients.Attach(ibs, sub.ClientID, []int{ib.Id})
			managerMarkRestart(needed)
			return err
		}); err != nil {
			return err
		}
		e.ActiveInboundID = ib.Id
		op.Completed = i + 1
		if err := db.Model(op).Update("completed", op.Completed).Error; err != nil {
			return err
		}
	}
	finish()
	if err := managerStep(op, "Applying destination configurations"); err != nil {
		return err
	}
	if err := s.applyGeneration(ctx, entries); err != nil {
		return err
	}
	if err := s.updateRelayPorts(ctx, entries); err != nil {
		return err
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err := managerStep(op, "Publishing the verified generation"); err != nil {
		return err
	}
	previous := sub.Published
	if len(previous) > 0 {
		grace := sub.GraceMinutes
		if grace == 0 {
			grace = 120
		}
		sub.Retired = append(sub.Retired, model.ManagerRetiredGeneration{Entries: previous, ExpiresAt: time.Now().Add(time.Duration(grace) * time.Minute).UnixMilli()})
	}
	sub.Entries, sub.Published = entries, entries
	sub.LastRotation = time.Now().UnixMilli()
	sub.NextRotation = time.Now().Add(time.Duration(sub.IntervalMinutes) * time.Minute).UnixMilli()
	if err := db.Save(sub).Error; err != nil {
		return err
	}
	published = true
	InvalidateSubData()
	active := managerKeep(sub)
	for _, e := range entries {
		active[e.ActiveInboundID] = true
	}
	if err := s.cleanup(ctx, op, sub, active); err != nil {
		return fmt.Errorf("%w: generation published: %w", errManagerCleanupPending, err)
	}
	relays := map[int]bool{}
	for _, e := range append(slices.Clone(previous), entries...) {
		if e.RelayID > 0 {
			relays[e.RelayID] = true
		}
	}
	for id := range relays {
		var r model.ManagerRelay
		if err := db.First(&r, id).Error; err != nil {
			return err
		}
		if r.Installed {
			if err := s.applyRelay(ctx, r, true, nil); err != nil {
				return fmt.Errorf("%w: generation published; relay: %w", errManagerCleanupPending, err)
			}
		}
	}
	op.Completed = op.Total
	return nil
}

func (s *ManagerService) clone(ctx context.Context, op *model.ManagerOperation, sub *model.ManagerSubscription, e model.ManagerEntry, index int) (*model.Inbound, error) {
	db := database.GetDB()
	svc := &InboundService{}
	src, err := managerEntrySource(e, sub.UserID)
	if err != nil {
		return nil, err
	}
	ib := &model.Inbound{UserId: sub.UserID, Enable: true, Protocol: src.Protocol, Settings: src.Settings, StreamSettings: src.StreamSettings, Sniffing: src.Sniffing, Listen: src.Listen, SubSortIndex: src.SubSortIndex, NodeID: src.NodeID, ShareAddrStrategy: src.ShareAddrStrategy, ShareAddr: src.ShareAddr, DisableFlow: src.DisableFlow, TrafficReset: "never", TrafficResetDay: 1, Remark: e.Flag + " " + e.Label, Tag: fmt.Sprintf("manager-%s-%d-%d", sub.Token, op.ID, index)}
	if e.NodeID != nil {
		ib.NodeID = e.NodeID
		if *e.NodeID == 0 {
			ib.NodeID = nil
		}
	}
	if ib.NodeID != nil {
		var node model.Node
		if err := db.First(&node, *ib.NodeID).Error; err != nil {
			return nil, err
		}
		if !node.Enable {
			return nil, fmt.Errorf("selected node is disabled")
		}
	}
	var settings map[string]any
	if err := json.Unmarshal([]byte(ib.Settings), &settings); err != nil {
		return nil, err
	}
	if settings == nil {
		settings = map[string]any{}
	}
	settings["clients"] = []any{}
	delete(settings, "peers")
	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, err
	}
	ib.Settings = string(raw)
	if err := managerFreshServerKeys(ib); err != nil {
		return nil, err
	}
	if err := s.applySNI(sub.UserID, ib, e.SNI); err != nil {
		return nil, err
	}
	// Recheck the selected random target and certificate, not only the preview sample.
	ib.Port, _ = strconv.Atoi(e.Port)
	if err := s.preflightTarget(ctx, ib); err != nil {
		return nil, err
	}
	if e.RelayID > 0 {
		var r model.ManagerRelay
		if err := db.First(&r, e.RelayID).Error; err != nil {
			return nil, err
		}
		ib.ShareAddrStrategy, ib.ShareAddr = "custom", r.Address
	}
	err = managerRetry(ctx, op, func() error {
		var old model.Inbound
		if err := db.Where("user_id = ? AND tag = ?", sub.UserID, ib.Tag).First(&old).Error; err == nil {
			*ib = old
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if e.Port == "random" || e.Port == "" {
			n, err := rand.Int(rand.Reader, big.NewInt(50000))
			if err != nil {
				return err
			}
			ib.Port = 10000 + int(n.Int64())
		} else {
			ib.Port, _ = strconv.Atoi(e.Port)
		}
		_, needed, err := svc.AddInbound(ib)
		managerMarkRestart(needed)
		return err
	})
	if err == nil && ib.NodeID != nil {
		err = managerRetry(ctx, op, func() error { return s.pushGenerated(ctx, ib) })
	}
	return ib, err
}

func (s *ManagerService) applySNI(owner int, ib *model.Inbound, sni string) error {
	if sni == "" {
		return nil
	}
	if sni == "random" {
		var transport struct {
			Security string `json:"security"`
		}
		if json.Unmarshal([]byte(ib.StreamSettings), &transport) != nil || transport.Security != "reality" {
			return fmt.Errorf("random SNI is only available for Reality; use the certificate domain for TLS")
		}
		var names []string
		if err := database.GetDB().Model(&model.ManagerDomain{}).Where("user_id = ?", owner).Pluck("name", &names).Error; err != nil {
			return err
		}
		if len(names) == 0 {
			return fmt.Errorf("SNI list is empty")
		}
		history, _ := SNIScanHistory()
		verified := map[string]bool{}
		for _, scan := range history {
			for _, row := range scan.Results {
				checked, err := time.Parse(time.RFC3339, row.CheckedAt)
				if err == nil && checked.After(time.Now().Add(-time.Hour)) && row.Feasible && row.CertValid {
					verified[row.Host] = true
				}
			}
		}
		preferred := []string{}
		for _, name := range names {
			if verified[name] {
				preferred = append(preferred, name)
			}
		}
		if len(preferred) > 0 {
			names = preferred
		}
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(names))))
		if err != nil {
			return err
		}
		sni = names[n.Int64()]
	}
	stream := map[string]any{}
	if strings.TrimSpace(ib.StreamSettings) != "" {
		if err := json.Unmarshal([]byte(ib.StreamSettings), &stream); err != nil {
			return err
		}
	}
	nativeUpdated := false
	var nativeSettings map[string]any
	if json.Unmarshal([]byte(ib.Settings), &nativeSettings) == nil {
		if tls, ok := nativeSettings["tls"].(map[string]any); ok {
			tls["serverName"] = sni
			raw, err := json.Marshal(nativeSettings)
			if err != nil {
				return err
			}
			ib.Settings = string(raw)
			nativeUpdated = true
		}
	}
	switch stream["security"] {
	case "reality":
		r, ok := stream["realitySettings"].(map[string]any)
		if !ok {
			return fmt.Errorf("Reality template required")
		}
		r["serverNames"], r["target"], r["dest"] = []string{sni}, sni+":443", sni+":443"
	case "tls":
		r, ok := stream["tlsSettings"].(map[string]any)
		if !ok {
			return fmt.Errorf("TLS template required")
		}
		r["serverName"] = sni
	default:
		if !nativeUpdated {
			return fmt.Errorf("SNI requires TLS or Reality")
		}
	}
	raw, err := json.Marshal(stream)
	if err == nil {
		ib.StreamSettings = string(raw)
	}
	return err
}

func (s *ManagerService) pushGenerated(ctx context.Context, ib *model.Inbound) error {
	svc := &InboundService{}
	rt, err := svc.runtimeFor(ib)
	if err != nil {
		return err
	}
	payload, err := svc.buildInboundForNodePush(database.GetDB(), ib)
	if err != nil {
		return err
	}
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return rt.UpdateInbound(c, payload, payload)
}

func (s *ManagerService) applyGeneration(ctx context.Context, entries []model.ManagerEntry) error {
	local := false
	nodes := map[int]*model.Inbound{}
	for _, e := range entries {
		if e.ActiveInboundID < 1 {
			continue
		}
		ib, err := (&InboundService{}).GetInbound(e.ActiveInboundID)
		if err != nil {
			return err
		}
		if ib.NodeID == nil {
			local = true
		} else {
			if err := s.pushGenerated(ctx, ib); err != nil {
				return err
			}
			nodes[*ib.NodeID] = ib
		}
	}
	for _, ib := range nodes {
		rt, err := (&InboundService{}).runtimeFor(ib)
		if err != nil {
			return err
		}
		c, cancel := context.WithTimeout(ctx, time.Minute)
		err = rt.RestartXray(c)
		cancel()
		if err != nil {
			return fmt.Errorf("node %d core apply: %w", *ib.NodeID, err)
		}
	}
	if !local {
		return nil
	}
	rt, err := (&InboundService{}).runtimeFor(&model.Inbound{})
	if err != nil {
		return err
	}
	c, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	if now, ok := rt.(interface{ ApplyConfigNow(context.Context) error }); ok {
		return now.ApplyConfigNow(c)
	}
	return rt.RestartXray(c)
}

func (s *ManagerService) cleanup(ctx context.Context, op *model.ManagerOperation, sub *model.ManagerSubscription, keep map[int]bool) error {
	var owned []model.Inbound
	if err := database.GetDB().Where("user_id = ? AND tag LIKE ?", sub.UserID, "%manager-"+sub.Token+"-%").Find(&owned).Error; err != nil {
		return err
	}
	for _, ib := range owned {
		prefix := "manager-" + sub.Token + "-"
		if !strings.HasPrefix(ib.Tag, prefix) && !strings.HasPrefix(ib.Tag, nodeTagPrefix(ib.NodeID)+prefix) {
			continue
		}
		if keep[ib.Id] {
			continue
		}
		if err := managerRetry(ctx, op, func() error {
			_, err := (&InboundService{}).GetInbound(ib.Id)
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			needed, err := (&InboundService{}).DelInbound(ib.Id)
			managerMarkRestart(needed)
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *ManagerService) deleteSubscription(ctx context.Context, op *model.ManagerOperation, sub *model.ManagerSubscription) error {
	db := database.GetDB()
	if err := db.Model(sub).Update("enabled", false).Error; err != nil {
		return err
	}
	if err := s.cleanup(ctx, op, sub, map[int]bool{}); err != nil {
		return err
	}
	if sub.OwnClient && sub.ClientID > 0 {
		needed, err := (&ClientService{}).Delete(&InboundService{}, sub.ClientID, false)
		managerMarkRestart(needed)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	relays := map[int]bool{}
	entries := append(slices.Clone(sub.Entries), sub.Published...)
	for _, g := range sub.Retired {
		entries = append(entries, g.Entries...)
	}
	for _, e := range entries {
		if e.RelayID > 0 {
			relays[e.RelayID] = true
		}
	}
	for id := range relays {
		var relay model.ManagerRelay
		if err := db.First(&relay, id).Error; err != nil {
			return err
		}
		if relay.Installed {
			if err := s.applyRelay(ctx, relay, true, nil, sub.ID); err != nil {
				return err
			}
		}
	}
	return db.Delete(sub).Error
}

func managerStep(op *model.ManagerOperation, message string) error {
	op.Message = message
	return database.GetDB().Model(op).Update("message", message).Error
}
