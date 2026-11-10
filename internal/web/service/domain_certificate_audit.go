package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/eventbus"
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
	NodeID int `json:"nodeId,omitempty"`
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
	if len(value) < 4 || len(value) > 253 || !strings.Contains(value, ".") || strings.Contains(value, "..") {
		return ""
	}
	host := strings.TrimPrefix(value, "*.")
	if strings.Contains(host, "*") {
		return ""
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return ""
		}
		for _, ch := range label {
			if (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' {
				continue
			}
			return ""
		}
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
		if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' {
			continue
		}
		id = ""
		break
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

// Node-specific audit entries are identified by the authenticated node's ID.
// Their private filesystem paths, warning text and credentials are excluded.
func RecordCertificateAuditWithNode(change CertificateAuditEntry) error {
	if change.NodeID <= 0 {
		return RecordCertificateAudit(change.Manager, change.CertificateID,
			change.Domain, change.Action, change.Outcome)
	}
	if change.NodeID > 100000000 || !strings.HasPrefix(change.CertificateID, "n") ||
		len(change.CertificateID) != 25 || change.Action != "health" {
		return errors.New("invalid remote certificate audit metadata")
	}
	switch change.Manager {
	case "certmagic", "acme.sh", "certbot", "manual", "external":
	default:
		return errors.New("invalid remote certificate manager")
	}
	switch change.Outcome {
	case "failed", "expiring", "expired", "recovered":
	default:
		return errors.New("invalid remote certificate health state")
	}
	for _, ch := range change.CertificateID {
		if ch >= 'a' && ch <= 'z' || ch >= '0' && ch <= '9' {
			continue
		}
		return errors.New("invalid remote certificate identifier")
	}
	change.At = time.Now().Unix()
	change.Domain = certificateAuditDomain(change.Domain)
	certificateAuditMu.Lock()
	defer certificateAuditMu.Unlock()
	entries, err := readCertificateAudit()
	if err != nil {
		return err
	}
	return saveFirewallJSON(certificateAuditSettingKey, boundedCertificateAudit(entries, change))
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
	return DomainCertificateAuditedEventsForDNS(httpPort, httpsPort, false)
}

// DNS-01 does not need public HTTP/HTTPS challenge ports; never acquire a
// temporary firewall lease for an issuance using a DNS-only solver.
func DomainCertificateAuditedEventsForDNS(httpPort, httpsPort int, dnsOnly bool) func(context.Context, string, map[string]any) error {
	firewallEvents := DomainCertificateFirewallEvents(httpPort, httpsPort)
	return func(ctx context.Context, event string, payload map[string]any) error {
		var err error
		if !dnsOnly {
			err = firewallEvents(ctx, event, payload)
		}
		name, _ := payload["identifier"].(string)
		if name != "" {
			action, outcome := "", ""
			operation := "obtain"
			if renewal, _ := payload["renewal"].(bool); renewal {
				operation = "renew"
			}
			switch event {
			case "cert_obtaining":
				action, outcome = operation, "started"
			case "cert_obtained":
				action, outcome = operation, "succeeded"
			case "cert_renewed":
				action, outcome = "renew", "succeeded"
			case "cert_failed":
				action, outcome = operation, "failed"
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
	if item.Manager == "remote" {
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
	// Retain remote health transitions while nodes are temporarily offline.
	// A missing node's absence is not proof that its certificate recovered.
	for id, state := range previous {
		if strings.HasPrefix(id, "n") && len(next) < 1024 {
			next[id] = state
		}
	}
	events := make([]CertificateAuditEntry, 0)
	for _, item := range items {
		if item.Manager == "remote" || item.ID == "" {
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
			Domain: item.Name, NodeID: item.NodeID, Action: "health", Outcome: outcome,
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
	remoteCtx, remoteCancel := context.WithTimeout(context.Background(), 6*time.Second)
	items = append(items, readRemoteCertificateInventories(remoteCtx)...)
	remoteCancel()
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
		if err := RecordCertificateAuditWithNode(change); err != nil {
			return err
		}
	}
	if err := saveFirewallJSON(certificateHealthSettingKey, next); err != nil {
		return err
	}
	// Persist transitions before notifying; a restart never replays the
	// same alert. Event bus subscribers decide whether Telegram/email is on.
	if eventBus != nil {
		for _, change := range changes {
			eventBus.Publish(eventbus.Event{
				Type: eventbus.EventCertificateHealth,
				Source: certificateAuditDomain(change.Domain),
				Data: &eventbus.CertificateHealthData{
					Manager: change.Manager, State: change.Outcome, NodeID: change.NodeID,
				},
			})
		}
	}
	return nil
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
