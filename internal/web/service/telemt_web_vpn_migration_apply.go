package service

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/mtproto"
	"github.com/SawaMEN/3x-ui/v3/internal/protocolserver"
)

const telemtVPNMigrationKey = "telemtVPNMigration"

// The apply workflow is limited to standalone WEB and supported native
// TLS listeners, with an authenticated probe for VLESS REALITY. Managed
// MTProto edits require additional lifecycle verification.
type TelemtVPNMigrationRequest struct {
	ID       int    `json:"id"`
	NewPort  int    `json:"newPort"`
	Revision string `json:"revision"`
}

type telemtVPNMigrationJournal struct {
	Moves                  []telemtVPNMigrationPlan `json:"moves"`
	Committed              bool                     `json:"committed"`
	ManagedInboundID       int                      `json:"managedInboundId,omitempty"`
	ManagedOriginalIngress *TelemtWebIngress        `json:"managedOriginalIngress,omitempty"`
}

func loadTelemtVPNMigration(db *gorm.DB) (*telemtVPNMigrationJournal, error) {
	var row model.Setting
	if err := db.Where("key = ?", telemtVPNMigrationKey).First(&row).Error; err != nil {
		if database.IsNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var journal telemtVPNMigrationJournal
	if err := json.Unmarshal([]byte(row.Value), &journal); err != nil {
		return nil, err
	}
	if journal.ManagedInboundID < 0 || (journal.ManagedInboundID > 0 && (journal.ManagedOriginalIngress == nil || !journal.ManagedOriginalIngress.Enabled)) {
		return nil, errors.New("invalid managed WEB migration journal")
	}
	if len(journal.Moves) == 0 || len(journal.Moves) > telemtIngressMaxInbounds {
		return nil, errors.New("invalid VPN migration journal")
	}
	seenIDs, seenPorts := map[int]bool{}, map[int]bool{}
	for _, move := range journal.Moves {
		if seenIDs[move.ID] || seenPorts[move.NewPort] {
			return nil, errors.New("duplicate VPN migration journal entry")
		}
		seenIDs[move.ID], seenPorts[move.NewPort] = true, true
		if move.ID == journal.ManagedInboundID || move.ID <= 0 || move.OldPort != 443 || move.NewListen != "127.0.0.1" || move.NewPort < telemtVPNMigrationFirstPort || move.NewPort > telemtVPNMigrationLastPort {
			return nil, errors.New("invalid VPN migration journal entry")
		}
		if move.TrustTunnelQUICBind != "" {
			if err := validateTelemtTrustTunnelMove(move); err != nil {
				return nil, err
			}
		}
		if move.VetroffPinH3 {
			host, port, err := net.SplitHostPort(move.VetroffH3Bind)
			if move.Protocol != string(model.VetroffProxy) || err != nil || net.ParseIP(host) == nil || port != "443" {
				return nil, errors.New("invalid VetrOFF HTTP/3 recovery entry")
			}
			if len(move.VetroffOldH3) > 0 {
				var old string
				if err := json.Unmarshal(move.VetroffOldH3, &old); err != nil || old != "" {
					return nil, errors.New("invalid previous VetrOFF HTTP/3 bind")
				}
			}
		}
	}
	return &journal, nil
}

func saveTelemtVPNMigration(tx *gorm.DB, journal *telemtVPNMigrationJournal) error {
	data, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	return saveSettingIn(tx, telemtVPNMigrationKey, string(data))
}

// Move only the socket columns, preserving tags, credentials, counters and
// client bindings. Recheck every exact revision, reserved port and fallback
// reference inside the same transaction as the recovery journal.
func stageTelemtVPNMigration(moves []telemtVPNMigrationPlan) error {
	return stageTelemtVPNMigrationWithDraft(moves, nil, nil)
}

func stageTelemtVPNMigrationWithDraft(moves []telemtVPNMigrationPlan, draft *model.Inbound, ingress *TelemtWebIngress) error {
	return runSerializedTx(func(tx *gorm.DB) error {
		if pending, err := loadTelemtVPNMigration(tx); err != nil {
			return err
		} else if pending != nil {
			return errors.New("another VPN migration is pending or active")
		}
		if len(moves) == 0 || len(moves) > telemtIngressMaxInbounds {
			return errors.New("invalid VPN migration selection")
		}
		seenIDs, seenPorts := map[int]bool{}, map[int]bool{}
		for _, move := range moves {
			if seenIDs[move.ID] || seenPorts[move.NewPort] || (draft != nil && draft.Id == move.ID) {
				return errors.New("duplicate VPN migration listener or port")
			}
			seenIDs[move.ID], seenPorts[move.NewPort] = true, true
			var inbound model.Inbound
			if err := tx.First(&inbound, move.ID).Error; err != nil {
				return err
			}
			if move.OldPort != 443 || move.NewListen != "127.0.0.1" || move.NewPort < telemtVPNMigrationFirstPort || move.NewPort > telemtVPNMigrationLastPort ||
				inbound.Port != move.OldPort || inbound.Listen != move.OldListen || telemtVPNMigrationRevision(inbound) != move.Revision {
				return fmt.Errorf("inbound %d changed; refresh migration preview", move.ID)
			}
			var references int64
			if err := tx.Model(&model.InboundFallback{}).Where("master_id = ? OR child_id = ?", move.ID, move.ID).Count(&references).Error; err != nil {
				return err
			}
			if references != 0 {
				return fmt.Errorf("inbound %d has dependent fallback rules", move.ID)
			}
			inbound.Listen, inbound.Port = move.NewListen, move.NewPort
			var err error
			inbound.Settings, err = patchTelemtVetroffH3(inbound.Settings, move, false)
			if err != nil {
				return err
			}
			inbound.Settings, err = patchTelemtTrustTunnelBind(inbound.Settings, move, false)
			if err != nil {
				return err
			}
			conflict, err := checkPortConflictTx(tx, &inbound, inbound.Id)
			if err != nil {
				return err
			}
			if conflict != nil {
				return errors.New(conflict.String())
			}
			updates := map[string]any{"listen": inbound.Listen, "port": inbound.Port}
			if move.VetroffPinH3 || move.TrustTunnelQUICBind != "" {
				updates["settings"] = inbound.Settings
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		journal := &telemtVPNMigrationJournal{Moves: moves}
		if draft != nil {
			if err := activateManagedMigrationDraftTx(tx, draft, ingress, journal); err != nil {
				return err
			}
		}
		return saveTelemtVPNMigration(tx, journal)
	})
}

func markTelemtVPNMigrationCommitted(committed bool) error {
	return runSerializedTx(func(tx *gorm.DB) error {
		journal, err := loadTelemtVPNMigration(tx)
		if err != nil {
			return err
		}
		if journal == nil {
			return errors.New("VPN migration journal missing")
		}
		journal.Committed = committed
		return saveTelemtVPNMigration(tx, journal)
	})
}

// Leave the journal in place until runtime restoration succeeds. Do not
// recreate a deleted inbound or overwrite an independently changed socket.
func restoreTelemtVPNMigrationRows() error {
	return runSerializedTx(func(tx *gorm.DB) error {
		journal, err := loadTelemtVPNMigration(tx)
		if err != nil || journal == nil {
			return err
		}
		if err := restoreManagedMigrationDraftTx(tx, journal); err != nil {
			return err
		}
		for _, move := range journal.Moves {
			var inbound model.Inbound
			if err := tx.First(&inbound, move.ID).Error; err != nil {
				if database.IsNotFound(err) {
					continue
				}
				return err
			}
			if inbound.NodeID != nil || string(inbound.Protocol) != move.Protocol || inbound.Listen != move.NewListen || inbound.Port != move.NewPort {
				continue
			}
			inbound.Listen, inbound.Port = move.OldListen, move.OldPort
			inbound.Settings, err = patchTelemtVetroffH3(inbound.Settings, move, true)
			if err != nil {
				return err
			}
			inbound.Settings, err = patchTelemtTrustTunnelBind(inbound.Settings, move, true)
			if err != nil {
				return err
			}
			conflict, err := checkPortConflictTx(tx, &inbound, inbound.Id)
			if err != nil {
				return err
			}
			if conflict != nil {
				return errors.New(conflict.String())
			}
			updates := map[string]any{"listen": inbound.Listen, "port": inbound.Port}
			if move.VetroffPinH3 || move.TrustTunnelQUICBind != "" {
				updates["settings"] = inbound.Settings
			}
			if err := tx.Model(&model.Inbound{}).Where("id = ?", inbound.Id).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func clearTelemtVPNMigration() error {
	return runSerializedTx(func(tx *gorm.DB) error {
		return tx.Where("key = ?", telemtVPNMigrationKey).Delete(&model.Setting{}).Error
	})
}

func (s TelemtService) enableWithVPNMigrationLocked(ctx context.Context, domain, cert, key string, options TelemtWebProxyOptions) (TelemtWebProxyStatus, error) {
	return s.enableWithVPNMigrationDraftLocked(ctx, domain, cert, key, options, nil)
}

func (s TelemtService) enableWithVPNMigrationDraftLocked(ctx context.Context, domain, cert, key string, options TelemtWebProxyOptions, draft *model.Inbound) (TelemtWebProxyStatus, error) {
	if os.Geteuid() != 0 || !s.IsInstalled() {
		return TelemtWebProxyStatus{}, errors.New("VPN migration requires installed Telemt and root privileges")
	}
	domain = normalizeTelemtWebDomain(domain)
	state, err := readTelemtWebState()
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	if state.Enabled || state.ManagedInboundID != 0 {
		return TelemtWebProxyStatus{}, errors.New("VPN migration requires an inactive WEB entry")
	}
	managed, err := mtprotoWebCandidates()
	if err != nil || len(managed) != 0 {
		return TelemtWebProxyStatus{}, errors.New("another managed MTProto WEB inbound is active")
	}
	if pending, err := TelemtPendingPortMigration(); err != nil || pending != nil {
		return TelemtWebProxyStatus{}, errors.New("finish the panel port migration first")
	}
	if options.SharedIngress == nil || !options.SharedIngress.Enabled {
		return TelemtWebProxyStatus{}, errors.New("enable shared ingress before migrating VPN listeners")
	}
	requested := slices.Clone(options.VPNMigrations)
	options.VPNMigrations = nil
	options, err = normalizeTelemtWebOptions(options)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	inboundID := 0
	if draft != nil {
		inboundID = draft.Id
	}
	preflight, err := s.PreflightWebProxyForInbound(ctx, domain, options, cert, key, inboundID)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	// The occupied TCP 443 is the only expected failure before relocation.
	for _, check := range preflight.Checks {
		if !check.OK && check.ID != "https-port" {
			return TelemtWebProxyStatus{}, fmt.Errorf("WEB preflight (%s): %s", check.ID, check.Message)
		}
	}
	if preflight.Preview == nil {
		return TelemtWebProxyStatus{}, errors.New("migration preview unavailable")
	}
	moves := preflight.Preview.VPNMigrations
	if len(moves) == 0 || len(moves) != len(requested) {
		return TelemtWebProxyStatus{}, errors.New("all TCP 443 reservations must be eligible and explicitly selected")
	}
	ids, err := telemtIngressInboundIDs(options.SharedIngress)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	for _, move := range moves {
		if move.Reason != "" || (move.Security != "tls" && (move.Security != "reality" || move.Protocol != string(model.VLESS))) {
			return TelemtWebProxyStatus{}, fmt.Errorf("inbound %d is not eligible for automatic migration", move.ID)
		}
		if !slices.ContainsFunc(requested, func(req TelemtVPNMigrationRequest) bool {
			return req.ID == move.ID && req.NewPort == move.NewPort && req.Revision == move.Revision
		}) {
			return TelemtWebProxyStatus{}, errors.New("migration preview changed; review the new plan")
		}
		ids = append(ids, move.ID)
	}
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) > telemtIngressMaxInbounds {
		return TelemtWebProxyStatus{}, errors.New("too many shared ingress inbounds")
	}
	options.SharedIngress = &TelemtWebIngress{Enabled: true, InboundIDs: ids}
	coreType, err := (&SettingService{}).GetCoreType()
	if err != nil || !IsNativeCore(coreType) {
		return TelemtWebProxyStatus{}, errors.New("migration requires the selected native core")
	}
	core := NativeCore(coreType)
	if !core.IsRunning() {
		return TelemtWebProxyStatus{}, errors.New("start the selected VPN core before migration")
	}
	if !telemtWebPortAvailable(443) {
		allowed := []string{core.BinaryPath()}
		if slices.ContainsFunc(moves, func(move telemtVPNMigrationPlan) bool {
			return move.Protocol == string(model.NaiveProxy)
		}) {
			allowed = append(allowed, protocolserver.BinaryPath(model.NaiveProxy))
		}
		if telemtMigrationHasVetroff(moves) {
			allowed = append(allowed, telemtMigrationVetroffBinary())
		}
		if telemtMigrationHasTrustTunnel(moves) {
			allowed = append(allowed, telemtMigrationTrustTunnelBinary())
		}
		if err := verifyTelemtVPNListenerOwners(ctx, allowed); err != nil {
			return TelemtWebProxyStatus{}, err
		}
	}
	realityProbes := make(map[int]map[string]any)
	shadowProbes := make(map[int]telemtShadowTLSProbe)
	for _, move := range moves {
		if move.Protocol == string(model.TrustTunnel) {
			if move.TrustTunnelQUICBind != "" {
				if err := externalvpn.CheckTrustTunnelQUICOnly(ctx); err != nil {
					return TelemtWebProxyStatus{}, fmt.Errorf("TrustTunnel cannot share TCP 443 with QUIC enabled; nothing was migrated: %w", err)
				}
			}
			if err := verifyTelemtTrustTunnelTransport(ctx, move, false); err != nil {
				return TelemtWebProxyStatus{}, err
			}
		}
		if move.Security == "reality" {
			var inbound model.Inbound
			if err := database.GetDB().First(&inbound, move.ID).Error; err != nil {
				return TelemtWebProxyStatus{}, err
			}
			profile, err := buildTelemtRealityMigrationProbe(&inbound, move)
			if err != nil {
				return TelemtWebProxyStatus{}, err
			}
			// Prove the original route works before any database or process changes.
			host := telemtRealityOriginalProbeHost(move.OldListen)
			if err := verifyTelemtRealityMigration(ctx, profile, host, move.OldPort, coreType); err != nil {
				return TelemtWebProxyStatus{}, fmt.Errorf("inbound %d: original REALITY client probe failed; nothing was migrated", move.ID)
			}
			realityProbes[move.ID] = profile
		}
		if move.Protocol == string(model.ShadowTLS) {
			var inbound model.Inbound
			if err := database.GetDB().First(&inbound, move.ID).Error; err != nil {
				return TelemtWebProxyStatus{}, err
			}
			profile, err := buildTelemtShadowTLSMigrationProbe(&inbound, move)
			if err != nil {
				return TelemtWebProxyStatus{}, err
			}
			if err := verifyTelemtShadowTLSMigration(ctx, profile, telemtRealityOriginalProbeHost(move.OldListen), move.OldPort, coreType); err != nil {
				return TelemtWebProxyStatus{}, fmt.Errorf("inbound %d: original ShadowTLS client probe failed; nothing was migrated", move.ID)
			}
			shadowProbes[move.ID] = profile
		}
		if err := verifyTelemtVetroffUDP(ctx, move); err != nil {
			return TelemtWebProxyStatus{}, err
		}
	}
	snapshot, err := s.CaptureWebProxyEnableSnapshot(domain)
	if err != nil {
		return TelemtWebProxyStatus{}, err
	}
	if err := stageTelemtVPNMigrationWithDraft(moves, draft, options.SharedIngress); err != nil {
		return TelemtWebProxyStatus{}, err
	}
	InvalidateSubData()
	rollback := func(cause error, frontendAttempted bool) (TelemtWebProxyStatus, error) {
		recovery, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		if frontendAttempted || draft != nil {
			current, readErr := readTelemtWebState()
			if readErr != nil {
				return TelemtWebProxyStatus{}, errors.Join(cause, readErr)
			}
			if current.Enabled || current.ManagedInboundID != 0 {
				if err := s.RestoreWebProxyEnableSnapshot(snapshot); err != nil {
					return TelemtWebProxyStatus{}, errors.Join(cause, err)
				}
			}
		}
		if err := restoreTelemtVPNMigrationRows(); err != nil {
			return TelemtWebProxyStatus{}, errors.Join(cause, err)
		}
		InvalidateSubData()
		if err := restartTelemtMigrationRuntime(recovery, core, moves); err != nil {
			return TelemtWebProxyStatus{}, errors.Join(cause, fmt.Errorf("restore VPN runtime: %w", err))
		}
		for _, move := range moves {
			if err := verifyTelemtVetroffUDP(recovery, move); err != nil {
				return TelemtWebProxyStatus{}, errors.Join(cause, err)
			}
			if move.TrustTunnelQUICBind != "" {
				if err := verifyTelemtTrustTunnelTransport(recovery, move, false); err != nil {
					return TelemtWebProxyStatus{}, errors.Join(cause, err)
				}
			}
		}
		return TelemtWebProxyStatus{}, errors.Join(cause, clearTelemtVPNMigration())
	}
	if err := restartTelemtMigrationRuntime(ctx, core, moves); err != nil {
		return rollback(err, false)
	}
	for _, move := range moves {
		dialer := net.Dialer{Timeout: 2 * time.Second}
		conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(move.NewListen, strconv.Itoa(move.NewPort)))
		if err != nil {
			return rollback(fmt.Errorf("migrated inbound %d is not listening: %w", move.ID, err), false)
		}
		_ = conn.Close()
	}
	if draft != nil {
		var active model.Inbound
		if err := database.GetDB().First(&active, draft.Id).Error; err != nil {
			return rollback(err, false)
		}
		runtime, err := (&InboundService{}).buildInboundForLocalRuntime(database.GetDB(), &active)
		if err != nil {
			return rollback(err, false)
		}
		if !mtprotoWebHasUsers(runtime) {
			return rollback(errors.New("managed MTProto WEB requires at least one active client"), false)
		}
		backendPort, err := mtproto.FreeLocalPort()
		if err != nil {
			return rollback(err, false)
		}
		state.ManagedInboundID = draft.Id
		state.BackendPort = backendPort
		state.UserFingerprint = mtprotoWebFingerprint(runtime)
		if err := writeTelemtWebState(state); err != nil {
			return rollback(err, false)
		}
	}
	result, err := s.enableWebProxyLocked(ctx, domain, cert, key, options)
	if err != nil {
		return rollback(err, true)
	}
	for _, move := range moves {
		var inbound model.Inbound
		if err := database.GetDB().First(&inbound, move.ID).Error; err != nil {
			return rollback(err, true)
		}
		if telemtIngressProxyHost(&inbound) == nil {
			return rollback(fmt.Errorf("migrated inbound %d was not published", move.ID), true)
		}
		if profile, ok := realityProbes[move.ID]; ok {
			if err := verifyTelemtRealityMigration(ctx, profile, move.NewListen, move.NewPort, coreType); err != nil {
				return rollback(fmt.Errorf("inbound %d: internal REALITY client probe failed", move.ID), true)
			}
			if err := verifyTelemtRealityMigration(ctx, profile, "127.0.0.1", 443, coreType); err != nil {
				return rollback(fmt.Errorf("inbound %d: shared REALITY client probe failed", move.ID), true)
			}
		} else if profile, ok := shadowProbes[move.ID]; ok {
			if err := verifyTelemtShadowTLSMigration(ctx, profile, move.NewListen, move.NewPort, coreType); err != nil {
				return rollback(fmt.Errorf("inbound %d: internal ShadowTLS client probe failed", move.ID), true)
			}
			if err := verifyTelemtShadowTLSMigration(ctx, profile, "127.0.0.1", 443, coreType); err != nil {
				return rollback(fmt.Errorf("inbound %d: shared ShadowTLS client probe failed", move.ID), true)
			}
		} else if err := verifyTelemtMigratedTLS(ctx, move); err != nil {
			return rollback(err, true)
		}
		if err := verifyTelemtVetroffUDP(ctx, move); err != nil {
			return rollback(err, true)
		}
		if move.Protocol == string(model.TrustTunnel) {
			if err := verifyTelemtTrustTunnelTransport(ctx, move, true); err != nil {
				return rollback(err, true)
			}
		}
	}
	if err := markTelemtVPNMigrationCommitted(true); err != nil {
		return rollback(err, true)
	}
	InvalidateSubData()
	return result, nil
}

// Recover a journal left by an interrupted apply before the VPN core starts.
// A committed journal remains available for the explicit return operation.
func RecoverInterruptedTelemtVPNMigration() error {
	journal, err := loadTelemtVPNMigration(database.GetDB())
	if err != nil || journal == nil || journal.Committed {
		return err
	}
	state, err := readTelemtWebState()
	if err != nil {
		return err
	}
	if state.Enabled || state.ManagedInboundID != 0 {
		if state.ManagedInboundID != 0 && state.ManagedInboundID != journal.ManagedInboundID {
			return errors.New("cannot recover migration over another managed WEB frontend")
		}
		if err := disableTelemtWebProxy(); err != nil {
			return err
		}
		if err := cleanupWebProxyBootUnit(); err != nil {
			return err
		}
	}
	if err := restoreTelemtVPNMigrationRows(); err != nil {
		return err
	}
	return clearTelemtVPNMigration()
}

func (s TelemtService) restoreVPNMigrationLocked() error {
	journal, err := loadTelemtVPNMigration(database.GetDB())
	if err != nil || journal == nil {
		return err
	}
	core := SelectedNativeCore()
	if !core.IsRunning() {
		return errors.New("start the selected core before returning VPN listeners")
	}
	if err := markTelemtVPNMigrationCommitted(false); err != nil {
		return err
	}
	if err := disableTelemtWebProxy(); err != nil {
		return errors.Join(err, markTelemtVPNMigrationCommitted(true))
	}
	if err := cleanupWebProxyBootUnit(); err != nil {
		return err
	}
	if err := restoreTelemtVPNMigrationRows(); err != nil {
		return err
	}
	InvalidateSubData()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := restartTelemtMigrationRuntime(ctx, core, journal.Moves); err != nil {
		return err
	}
	// Verify only unchanged VetrOFF listeners: an administrator may have
	// deliberately removed or reconfigured an inbound since migration.
	for _, move := range journal.Moves {
		if move.TrustTunnelQUICBind != "" {
			var row model.Inbound
			if err := database.GetDB().First(&row, move.ID).Error; err != nil {
				if database.IsNotFound(err) {
					continue
				}
				return err
			}
			if row.Enable && row.Protocol == model.TrustTunnel && row.Port == move.OldPort && row.Listen == move.OldListen {
				if err := verifyTelemtTrustTunnelTransport(ctx, move, false); err != nil {
					return err
				}
			}
		}
		if move.VetroffH3Bind == "" {
			continue
		}
		var inbound model.Inbound
		if err := database.GetDB().First(&inbound, move.ID).Error; err != nil {
			if database.IsNotFound(err) {
				continue
			}
			return err
		}
		if inbound.Protocol != model.VetroffProxy || !inbound.Enable || inbound.Port != move.OldPort || inbound.Listen != move.OldListen {
			continue
		}
		current := telemtVPNMigrationPlan{}
		if err := planTelemtVetroffH3(inbound, &current); err != nil {
			return err
		}
		if current.VetroffH3Bind == move.VetroffH3Bind {
			if err := verifyTelemtVetroffUDP(ctx, move); err != nil {
				return err
			}
		}
	}
	return clearTelemtVPNMigration()
}

func loadTelemtVPNMigrationForEnable() (*telemtVPNMigrationJournal, error) {
	db := database.GetDB()
	if db == nil {
		return nil, errors.New("database unavailable")
	}
	return loadTelemtVPNMigration(db)
}

// Compare the backend certificate to the certificate observed via the shared
// listener. This is a local routing probe, not a public trust or VPN
// authentication test. A distinct SNI must reach the same TLS endpoint.
func verifyTelemtMigratedTLS(ctx context.Context, move telemtVPNMigrationPlan) error {
	probe, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	leaf := func(address string, expected []byte) ([]byte, error) {
		// Both destinations are explicit loopback addresses. The backend
		// certificate is pinned for the second handshake rather than requiring
		// a publicly trusted CA on the private test endpoint.
		var certificate []byte
		dialer := tls.Dialer{
			NetDialer: &net.Dialer{Timeout: 3 * time.Second},
			Config: &tls.Config{
				MinVersion:         tls.VersionTLS12,
				ServerName:         move.SNI,
				NextProtos:         []string{"h2", "http/1.1"},
				InsecureSkipVerify: true, // verified against the local backend below
				VerifyConnection: func(state tls.ConnectionState) error {
					if len(state.PeerCertificates) == 0 {
						return errors.New("missing TLS peer certificate")
					}
					certificate = state.PeerCertificates[0].Raw
					if expected != nil && !bytes.Equal(certificate, expected) {
						return errors.New("public listener returned a different certificate")
					}
					return nil
				},
			},
		}
		conn, err := dialer.DialContext(probe, "tcp", address)
		if err != nil {
			return nil, err
		}
		_ = conn.Close()
		return certificate, nil
	}
	cert, err := leaf(net.JoinHostPort(move.NewListen, strconv.Itoa(move.NewPort)), nil)
	if err == nil {
		_, err = leaf("127.0.0.1:443", cert)
	}
	if err != nil {
		return fmt.Errorf("verify shared TLS route for inbound %d: %w", move.ID, err)
	}
	return nil
}

// Check actual executable paths, not just process names. An unrelated caddy
// or sing-box process on the same server must not be treated as ours.
var telemtMigrationPIDPattern = regexp.MustCompile(`pid=([0-9]+)`)

func verifyTelemtVPNListenerOwners(ctx context.Context, allowed []string) error {
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probe, "ss", "-lntpH", "sport = :443").Output()
	if err != nil {
		return fmt.Errorf("inspect TCP 443 processes: %w", err)
	}
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	return validateTelemtVPNListenerOwners(lines, allowed, func(pid string) (string, error) {
		return os.Readlink(filepath.Join("/proc", pid, "exe"))
	})
}

func validateTelemtVPNListenerOwners(lines, allowed []string, executable func(string) (string, error)) error {
	paths := make(map[string]bool, len(allowed))
	for _, path := range allowed {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		if resolved, err := filepath.EvalSymlinks(absolute); err == nil {
			absolute = resolved
		}
		paths[absolute] = true
	}
	found := false
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		matches := telemtMigrationPIDPattern.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			return errors.New("TCP 443 listener owner cannot be verified")
		}
		for _, match := range matches {
			path, err := executable(match[1])
			if err != nil || !paths[path] {
				return errors.New("TCP 443 has a listener outside the selected core or managed Naive binary")
			}
			found = true
		}
	}
	if !found {
		return errors.New("TCP 443 owner could not be established")
	}
	return nil
}

func telemtMigrationVetroffBinary() string {
	return filepath.Join(config.GetBinFolderPath(), "vetroff-bundle", "vetroff-necko", "vetroff")
}

func telemtMigrationHasVetroff(moves []telemtVPNMigrationPlan) bool {
	return slices.ContainsFunc(moves, func(move telemtVPNMigrationPlan) bool {
		return move.Protocol == string(model.VetroffProxy)
	})
}

func restartTelemtMigrationRuntime(ctx context.Context, core *SingBoxService, moves []telemtVPNMigrationPlan) error {
	if err := core.Restart(ctx); err != nil {
		return err
	}
	if telemtMigrationHasVetroff(moves) || telemtMigrationHasTrustTunnel(moves) {
		// Resolve the committed desired set under the sidecar manager lock.
		// Do not bypass disabled-client and quota filtering.
		_, err := externalvpn.GetManager().ReconcileCurrent(ctx, (&InboundService{}).DesiredExternalVPNInstancesContext)
		return err
	}
	return nil
}

func verifyTelemtVetroffUDP(ctx context.Context, move telemtVPNMigrationPlan) error {
	if move.Protocol != string(model.VetroffProxy) || move.VetroffH3Bind == "" {
		return nil
	}
	host, port, err := net.SplitHostPort(move.VetroffH3Bind)
	if err != nil {
		return err
	}
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probe, "ss", "-lunpH", "sport = :"+port).Output()
	if err != nil {
		return fmt.Errorf("inspect VetrOFF HTTP/3 socket: %w", err)
	}
	lines := []string{}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		boundHost, boundPort, err := net.SplitHostPort(fields[3])
		if err != nil || boundPort != port {
			continue
		}
		expected := net.ParseIP(host)
		actual := net.ParseIP(boundHost)
		if expected != nil && (expected.Equal(actual) || expected.IsUnspecified() && boundHost == "*") {
			lines = append(lines, line)
		}
	}
	if err := validateTelemtVPNListenerOwners(lines, []string{telemtMigrationVetroffBinary()}, func(pid string) (string, error) {
		return os.Readlink(filepath.Join("/proc", pid, "exe"))
	}); err != nil {
		return fmt.Errorf("VetrOFF HTTP/3 socket %s not verified: %w", move.VetroffH3Bind, err)
	}
	return nil
}

func telemtMigrationHasTrustTunnel(moves []telemtVPNMigrationPlan) bool {
	return slices.ContainsFunc(moves, func(move telemtVPNMigrationPlan) bool {
		return move.Protocol == string(model.TrustTunnel)
	})
}

func telemtMigrationTrustTunnelBinary() string {
	return filepath.Join(config.GetBinFolderPath(), "trusttunnel_endpoint")
}

// A saved TCP-only setting is insufficient if the previous process still
// owns a QUIC socket. Other protocols may independently own UDP 443.
func verifyTelemtTrustTunnelTCPOnly(ctx context.Context, port int) error {
	probe, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(probe, "ss", "-lunpH", "sport = :"+strconv.Itoa(port)).Output()
	if err != nil {
		return fmt.Errorf("inspect TrustTunnel UDP sockets: %w", err)
	}
	return validateTelemtTrustTunnelNoUDP(string(output), func(pid string) (string, error) {
		return os.Readlink(filepath.Join("/proc", pid, "exe"))
	})
}

func validateTelemtTrustTunnelNoUDP(output string, executable func(string) (string, error)) error {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		matches := telemtMigrationPIDPattern.FindAllStringSubmatch(line, -1)
		if len(matches) == 0 {
			return errors.New("UDP listener owner cannot be verified")
		}
		for _, match := range matches {
			path, err := executable(match[1])
			if err != nil {
				return errors.New("UDP listener executable cannot be verified")
			}
			if strings.Contains(filepath.Base(path), "trusttunnel_endpoint") {
				return errors.New("TrustTunnel still owns a QUIC socket; apply TCP-only settings before migration")
			}
		}
	}
	return nil
}
