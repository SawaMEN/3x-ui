package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

const (
	managerSNIHealthFailureThreshold = 3
	managerSNIHealthCooldown         = 30 * time.Minute
	managerSNIHealthMaxPerOperation  = 8
	managerSNIHealthProbeTimeout     = 75 * time.Second
)

// managerHealthySNI never relies on a successful probe to an unrelated IP.
// Automatic live failover requires the candidate's DNS to resolve to the
// same IP whose TLS certificate and handshake were verified, repeatedly.
func managerHealthySNI(row *RealityScanResult, hostname string) bool {
	return row != nil &&
		strings.EqualFold(row.Host, hostname) && row.Port == 443 &&
		row.Feasible && row.CertValid && row.TLS13 && row.H2 && row.X25519 &&
		!row.PrivateTarget && row.DNSMatchesIP != nil && *row.DNSMatchesIP &&
		row.Attempts >= 4 && row.Successes*5 >= row.Attempts*4
}

func managerSNIHealthDecision(entry model.ManagerEntry, healthy bool, now int64) (int, bool) {
	if healthy {
		return 0, false
	}
	failures := min(entry.SNIHealthFailures+1, managerSNIHealthFailureThreshold)
	if failures < managerSNIHealthFailureThreshold {
		return failures, false
	}
	if entry.SNIHealthLastSwitchAt > 0 &&
		now-entry.SNIHealthLastSwitchAt < managerSNIHealthCooldown.Milliseconds() {
		return failures, false
	}
	return failures, true
}

func managerSNIHealthBackups(entry model.ManagerEntry, selected string) []string {
	// Keep the previous working hostname available for a later recovery.
	choices := make([]string, 0, len(entry.SNIBackups)+1)
	for _, name := range append(slices.Clone(entry.SNIBackups), entry.SNI) {
		if name != selected && !slices.Contains(choices, name) {
			choices = append(choices, name)
		}
	}
	if len(choices) > 6 {
		choices = choices[:6]
	}
	return choices
}

// scanManagerSNI reuses the existing bounded scanner, always on the physical
// node that hosts the inbound. Scanner/network failures do not count as proof
// of an unhealthy destination: they must not trigger a switch.
func scanManagerSNI(ctx context.Context, inbound *model.Inbound, hostname string) (bool, error) {
	hostname, err := managerDomain(hostname)
	if err != nil {
		return false, err
	}
	probeCtx, cancel := context.WithTimeout(ctx, managerSNIHealthProbeTimeout)
	defer cancel()
	if inbound.NodeID == nil {
		svc := &ServerService{}
		rows, err := svc.ScanRealityTargetsContext(probeCtx, hostname+":443")
		if err != nil {
			return false, err
		}
		for _, row := range rows {
			if row == nil || !strings.EqualFold(row.Host, hostname) || !row.Feasible || !row.CertValid {
				continue
			}
			measured := verifySNICandidate(probeCtx, svc, row, "reality")
			if managerHealthySNI(measured, hostname) {
				return true, nil
			}
		}
		return false, probeCtx.Err()
	}
	rt, err := (&InboundService{}).runtimeFor(inbound)
	if err != nil {
		return false, err
	}
	scanner, ok := rt.(runtime.SNIJobRuntime)
	if !ok {
		return false, fmt.Errorf("SNI scanner is unavailable on node %d", *inbound.NodeID)
	}
	raw, err := scanner.RequestSNIScan(probeCtx, http.MethodPost, "jobs", SNIScanRequest{
		Mode: "candidates", Targets: hostname + ":443", Purpose: "reality",
	})
	if err != nil {
		return false, err
	}
	var job SNIJobSnapshot
	if err := json.Unmarshal(raw, &job); err != nil {
		return false, err
	}
	if job.ID == "" {
		return false, errors.New("remote SNI scanner did not return a job ID")
	}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		switch job.State {
		case "done":
			for _, row := range job.Results {
				if managerHealthySNI(row, hostname) {
					return true, nil
				}
			}
			return false, nil
		case "failed", "canceled":
			return false, fmt.Errorf("remote SNI scan failed: %s", job.Error)
		}
		select {
		case <-probeCtx.Done():
			return false, probeCtx.Err()
		case <-ticker.C:
			raw, err = scanner.RequestSNIScan(probeCtx, http.MethodGet, "jobs/"+job.ID, nil)
			if err != nil {
				return false, err
			}
			if err := json.Unmarshal(raw, &job); err != nil {
				return false, err
			}
		}
	}
}

func managerSNIHealthEligible(entry model.ManagerEntry) bool {
	return entry.ActiveInboundID > 0 && len(entry.SNIBackups) > 0 && entry.RealityTarget == "" &&
		(entry.Mode == "generate" || entry.Mode == "rotate") &&
		entry.SNI != "" && entry.SNI != "random"
}

// monitorSNI is a queued, per-subscription operation. It never deletes or
// re-creates a listener. After three independent failed scheduled checks,
// select a repeatedly verified backup and update only its REALITY camouflage
// fields. The existing client credentials, port, private key and short IDs
// are retained. On any apply/persistence error the original inbound is
// restored and the currently published subscription stays unchanged.
func (s *ManagerService) monitorSNI(ctx context.Context, op *model.ManagerOperation, sub *model.ManagerSubscription) error {
	if !sub.MonitorSNI || !sub.Enabled || len(sub.Published) == 0 {
		op.Message = "SNI monitoring disabled or not yet published"
		return nil
	}
	indices := make([]int, 0, len(sub.Published))
	for i, entry := range sub.Published {
		if managerSNIHealthEligible(entry) {
			indices = append(indices, i)
		}
	}
	slices.SortStableFunc(indices, func(a, b int) int {
		ta, tb := sub.Published[a].SNIHealthCheckedAt, sub.Published[b].SNIHealthCheckedAt
		switch {
		case ta < tb:
			return -1
		case ta > tb:
			return 1
		default:
			return a - b
		}
	})
	if len(indices) > managerSNIHealthMaxPerOperation {
		indices = indices[:managerSNIHealthMaxPerOperation]
	}
	op.Total = len(indices)
	op.Completed = 0
	svc := &InboundService{}
	checked, failed, switched := 0, 0, 0
	for _, index := range indices {
		if err := ctx.Err(); err != nil {
			return err
		}
		entry := &sub.Published[index]
		ib, err := svc.GetInbound(entry.ActiveInboundID)
		if err != nil {
			return err
		}
		if ib.UserId != sub.UserID || !strings.HasPrefix(ib.Tag, "manager-"+sub.Token+"-") ||
			!ib.Enable {
			return fmt.Errorf("SNI monitor refuses inbound %d not owned by its generation", ib.Id)
		}
		var stream struct {
			Security string `json:"security"`
			Reality  struct {
				ServerNames []string `json:"serverNames"`
			} `json:"realitySettings"`
		}
		if err := json.Unmarshal([]byte(ib.StreamSettings), &stream); err != nil || stream.Security != "reality" ||
			len(stream.Reality.ServerNames) != 1 || !strings.EqualFold(stream.Reality.ServerNames[0], entry.SNI) {
			return fmt.Errorf("SNI monitor refuses changed REALITY inbound %d", ib.Id)
		}
		ok, err := scanManagerSNI(ctx, ib, entry.SNI)
		if err != nil {
			return fmt.Errorf("SNI scanner unavailable on inbound %d: %w", ib.Id, err)
		}
		now := time.Now().UnixMilli()
		count, shouldSwitch := managerSNIHealthDecision(*entry, ok, now)
		entry.SNIHealthFailures = count
		entry.SNIHealthCheckedAt = now
		checked++
		if !ok {
			failed++
		}
		if shouldSwitch {
			for _, backup := range entry.SNIBackups {
				ok, err := scanManagerSNI(ctx, ib, backup)
				if err != nil {
					return fmt.Errorf("backup SNI scanner unavailable on inbound %d: %w", ib.Id, err)
				}
				if !ok {
					continue
				}
				original := *ib
				next := *ib
				proposed := *entry
				proposed.SNI = backup
				if err := s.applyEntrySNI(sub.UserID, &next, proposed); err != nil {
					return err
				}
				if original.Settings != next.Settings || original.Port != next.Port ||
					!managerSNIHealthRealityKeysEqual(original.StreamSettings, next.StreamSettings) {
					return fmt.Errorf("SNI monitor refuses credential, port or REALITY key changes")
				}
				_, restart, err := svc.UpdateInbound(&next)
				if err != nil {
					return err
				}
				managerMarkRestart(restart)
				applied := s.applyGeneration(ctx, []model.ManagerEntry{{ActiveInboundID: next.Id}})
				if applied != nil {
					return s.rollbackManagerSNIHealth(ctx, &original, applied)
				}
				before := *entry
				entry.SNI, entry.SNIBackups = backup, managerSNIHealthBackups(before, backup)
				entry.SNIHealthFailures, entry.SNIHealthLastSwitchAt = 0, now
				for i := range sub.Entries {
					if sub.Entries[i].ActiveInboundID == entry.ActiveInboundID ||
						i == index && sub.Entries[i].Preset == entry.Preset && sub.Entries[i].Mode == entry.Mode {
						sub.Entries[i].SNI = entry.SNI
						sub.Entries[i].SNIBackups = slices.Clone(entry.SNIBackups)
						break
					}
				}
				if err := database.GetDB().Save(sub).Error; err != nil {
					*entry = before
					return s.rollbackManagerSNIHealth(ctx, &original, err)
				}
				InvalidateSubData()
				switched++
				// One live change per job gives each operation one bounded,
				// easily auditable rollback scope.
				break
			}
		}
		op.Completed++
		if switched > 0 {
			break
		}
	}
	if err := database.GetDB().Save(sub).Error; err != nil {
		return err
	}
	op.Message = fmt.Sprintf("SNI health: checked=%d unhealthy=%d switched=%d", checked, failed, switched)
	return nil
}

func (s *ManagerService) rollbackManagerSNIHealth(ctx context.Context, original *model.Inbound, cause error) error {
	restoreCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	svc := &InboundService{}
	_, restart, restoreErr := svc.UpdateInbound(original)
	managerMarkRestart(restart)
	if restoreErr == nil {
		restoreErr = s.applyGeneration(restoreCtx, []model.ManagerEntry{{ActiveInboundID: original.Id}})
	}
	if restoreErr != nil {
		return fmt.Errorf("SNI switch failed: %w; rollback failed: %v", cause, restoreErr)
	}
	return fmt.Errorf("SNI switch failed and was rolled back: %w", cause)
}

func managerSNIHealthRealityKeysEqual(left, right string) bool {
	var a, b struct {
		Security string `json:"security"`
		Reality  struct {
			PrivateKey string   `json:"privateKey"`
			ShortIDs   []string `json:"shortIds"`
		} `json:"realitySettings"`
	}
	if json.Unmarshal([]byte(left), &a) != nil || json.Unmarshal([]byte(right), &b) != nil {
		return false
	}
	return a.Security == "reality" && b.Security == "reality" &&
		a.Reality.PrivateKey == b.Reality.PrivateKey && slices.Equal(a.Reality.ShortIDs, b.Reality.ShortIDs)
}
