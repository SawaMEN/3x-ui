package service

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

const (
	certificateAuditSettingKey = "domainCertificateAuditHistory"
	certificateHealthSettingKey = "domainCertificateHealthStates"
	certificateAuditLimit = 250
)

var (
	certificateAuditMu sync.Mutex
	certificateHealthMu sync.Mutex
	auditDomainPattern = regexp.MustCompile(`^(?:\\*\\.)?[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)
)

// CertificateAuditEntry stores only fixed event types and a trusted local
// certificate identifier. No command output, tokens, paths or ACME payloads
// are ever retained in this history.
type CertificateAuditEntry struct {
	At int64 `json:"at"`
	Manager string `json:"manager"`
	CertificateID string `json:"certificateId,omitempty"`
	Domain string `json:"domain,omitempty"`
	Action string `json:"action"`
	Outcome string `json:"outcome"`
}

func boundedCertificateAudit(entries []CertificateAuditEntry, event CertificateAuditEntry) []CertificateAuditEntry {
	entries = append(entries, event)
	if len(entries) > certificateAuditLimit {
		entries = entries[len(entries)-certificateAuditLimit:]
	}
	return entries
}

func certificateAuditDomain(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.TrimSuffix(value, ".")))
	if len(value) > 253 || !auditDomainPattern.MatchString(value) || !strings.Contains(value, ".") || strings.Contains(value, "..") {
		return ""
	}
	return value
}

func readCertificateAudit() ([]CertificateAuditEntry, error) {
	raw, err := firewallSetting(certificateAuditSettingKey, "[]")
	if err != nil {
		return nil, err
	}
	var entries []CertificateAuditEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, err
	}
	if entries == nil {
		entries = []CertificateAuditEntry{}
	}
	if len(entries) > certificateAuditLimit {
		entries = entries[len(entries)-certificateAuditLimit:]
	}
	return entries, nil
}

func RecordCertificateAudit(manager, id, domain, action, outcome string) error {
	switch manager {
	case "certmagic", "acme.sh", "certbot", "manual", "external":
	default:
		return errors.New("unrecognized certificate manager")
	}
	switch action {
	case "obtain", "renew", "hook", "health":
	default:
		return errors.New("unrecognized certificate action")
	}
	switch outcome {
	case "started", "succeeded", "failed", "expiring", "expired", "recovered":
	default:
		return errors.New("unrecognized certificate outcome")
	}
	if len(id) > 128 {
		id = ""
	}
	for _, ch := range id {
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
			id = ""
			break
		}
	}
	event := CertificateAuditEntry{
		At: time.Now().Unix(), Manager: manager, CertificateID: id,
		Domain: certificateAuditDomain(domain), Action: action, Outcome: outcome,
	}
	certificateAuditMu.Lock()
	defer certificateAuditMu.Unlock()
	entries, err := readCertificateAudit()
	if err != nil {
		return err
	}
	return saveFirewallJSON(certificateAuditSettingKey, boundedCertificateAudit(entries, event))
}

func CertificateAuditHistory() ([]CertificateAuditEntry, error) {
	certificateAuditMu.Lock()
	defer certificateAuditMu.Unlock()
	entries, err := readCertificateAudit()
	if err != nil {
		return nil, err
	}
	for i, j := 0, len(entries)-1; i < j; i, j = i+1, j-1 {
		entries[i], entries[j] = entries[j], entries[i]
	}
	return entries, nil
}

// Keep CertMagic event journaling separate from temporary firewall leases.
// An audit failure must never interrupt certificate issuance or renewal.
func DomainCertificateAuditedEvents(httpPort, httpsPort int) func(context.Context, string, map[string]any) error {
	firewallEvents := DomainCertificateFirewallEvents(httpPort, httpsPort)
	return func(ctx context.Context, event string, payload map[string]any) error {
		err := firewallEvents(ctx, event, payload)
		name, _ := payload["identifier"].(string)
		if name != "" {
			action, outcome := "", ""
			switch event {
			case "cert_obtaining":
				action, outcome = "obtain", "started"
			case "cert_obtained":
				action, outcome = "obtain", "succeeded"
			case "cert_renewed":
				action, outcome = "renew", "succeeded"
			case "cert_failed":
				action, outcome = "obtain", "failed"
			}
			if err != nil {
				outcome = "failed"
			}
			if action != "" {
				if auditErr := RecordCertificateAudit("certmagic", "", name, action, outcome); auditErr != nil {
					logger.Debug("certificate audit write failed:", auditErr)
				}
			}
		}
		return err
	}
}

// certificateHealthState is intentionally independent of the manager.
// A remote node's certificate is never read or renewed by this scanner.
func certificateHealthState(item CertificateInventoryItem, now time.Time) string {
	if item.Manager == "remote" || item.NodeID > 0 {
		return ""
	}
	if item.State == "error" || item.State == "invalid" {
		return "failed"
	}
	if item.ExpiresAt <= 0 {
		return ""
	}
	expires := time.Unix(item.ExpiresAt, 0)
	if !expires.After(now) {
		return "expired"
	}
	if !expires.After(now.Add(30 * 24 * time.Hour)) {
		return "expiring"
	}
	return ""
}

func changedCertificateHealth(previous map[string]string, items []CertificateInventoryItem, now time.Time) (map[string]string, []CertificateAuditEntry) {
	next := make(map[string]string)
	events := make([]CertificateAuditEntry, 0)
	for _, item := range items {
		if item.Manager == "remote" || item.NodeID > 0 || item.ID == "" {
			continue
		}
		state := certificateHealthState(item, now)
		if state != "" {
			next[item.ID] = state
		}
		before := previous[item.ID]
		if before == state || before == "" && state == "" {
			continue
		}
		outcome := state
		if state == "" {
			outcome = "recovered"
		}
		events = append(events, CertificateAuditEntry{
			Manager: item.Manager, CertificateID: item.ID,
			Domain: item.Name, Action: "health", Outcome: outcome,
		})
	}
	return next, events
}

func scanCertificateHealth() error {
	certificateHealthMu.Lock()
	defer certificateHealthMu.Unlock()
	status, err := DomainFrontStatus()
	if err != nil {
		return err
	}
	items := makeCertificateInventory(status, domainACMEStates(domainACMEHomes()), nil)
	raw, err := firewallSetting(certificateHealthSettingKey, "{}")
	if err != nil {
		return err
	}
	previous := map[string]string{}
	if err := json.Unmarshal([]byte(raw), &previous); err != nil {
		return err
	}
	next, changes := changedCertificateHealth(previous, items, time.Now())
	for _, change := range changes {
		if err := RecordCertificateAudit(change.Manager, change.CertificateID, change.Domain, change.Action, change.Outcome); err != nil {
			return err
		}
	}
	return saveFirewallJSON(certificateHealthSettingKey, next)
}

// RunCertificateHealthMonitor uses a bounded periodic scan and emits events
// only when a local certificate changes health, avoiding repeated notifications.
func RunCertificateHealthMonitor(ctx context.Context) {
	timer := time.NewTimer(2 * time.Minute)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if err := scanCertificateHealth(); err != nil {
				logger.Debug("certificate health scan failed:", err)
			}
			timer.Reset(time.Hour)
		}
	}
}

var _ = domainfront.DomainFrontStatus{}
