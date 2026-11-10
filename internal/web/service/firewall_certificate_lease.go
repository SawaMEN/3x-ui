package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

const (
	firewallRenewalLeasesKey = "firewallCertificateRenewalLeases"
	firewallRenewalLeaseTTL = 20 * time.Minute
)

// A new process ID invalidates stale rules immediately after a service crash.
var firewallRenewalProcess = newRenewalLeaseID()

func newRenewalLeaseID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw[:])
}

type firewallRenewalLease struct {
	ID        string `json:"id"`
	Process   string `json:"process"`
	Owner     string `json:"owner"`
	Ports     []int  `json:"ports"`
	ExpiresAt int64  `json:"expiresAt"`
}

func validRenewalPorts(ports []int) bool {
	for _, port := range ports {
		if port != 80 && port != 443 {
			return false
		}
	}
	return true
}

func loadFirewallRenewalLeases() ([]firewallRenewalLease, error) {
	raw, err := firewallSetting(firewallRenewalLeasesKey, "[]")
	if err != nil {
		return nil, err
	}
	var leases []firewallRenewalLease
	if err := json.Unmarshal([]byte(raw), &leases); err != nil {
		return nil, fmt.Errorf("renewal firewall lease state: %w", err)
	}
	return leases, nil
}

func activeRenewalLeases(all []firewallRenewalLease, now time.Time) []firewallRenewalLease {
	out := make([]firewallRenewalLease, 0, len(all))
	for _, lease := range all {
		if lease.Process == firewallRenewalProcess && lease.ID != "" && lease.ExpiresAt > now.Unix() && validRenewalPorts(lease.Ports) {
			out = append(out, lease)
		}
	}
	return out
}

func firewallRenewalRules(all []firewallRenewalLease, now time.Time) []FirewallRule {
	seen := map[int]bool{}
	out := []FirewallRule{}
	for _, lease := range activeRenewalLeases(all, now) {
		for _, port := range lease.Ports {
			if seen[port] {
				continue
			}
			seen[port] = true
			out = append(out, FirewallRule{
				Port: port, Protocol: "tcp", Source: "certificate-renewal",
				Label: "Temporary TLS challenge",
			})
		}
	}
	return out
}

// Begin makes the lease part of the desired rules before returning to the
// certificate issuer. It never turns on a firewall disabled by the operator.
func RenewalFirewallBegin(ctx context.Context, owner string, ports []int) (string, error) {
	if !validRenewalPorts(ports) {
		return "", errors.New("renewal only permits TCP 80 and TCP 443")
	}
	if len(ports) == 0 {
		return "", nil
	}
	firewallMu.Lock()
	defer firewallMu.Unlock()
	if !firewallControlInitialized() {
		return "", nil
	}
	enabled, configured, err := firewallManagedEnabledPreference()
	if err != nil {
		return "", err
	}
	if configured && !enabled {
		return "", nil
	}
	backend, err := detectManagedFirewallBackend(ctx)
	if err != nil {
		return "", fmt.Errorf("certificate challenge firewall: %w", err)
	}
	on, err := backend.enabled(ctx)
	if err != nil {
		return "", err
	}
	if !on && !(configured && enabled && isManagedNativeFirewall(backend.name)) {
		return "", nil
	}
	all, err := loadFirewallRenewalLeases()
	if err != nil {
		return "", err
	}
	active := activeRenewalLeases(all, time.Now())
	id := newRenewalLeaseID()
	active = append(active, firewallRenewalLease{
		ID: id, Process: firewallRenewalProcess, Owner: owner,
		Ports: slices.Clone(ports), ExpiresAt: time.Now().Add(firewallRenewalLeaseTTL).Unix(),
	})
	if err := saveFirewallJSON(firewallRenewalLeasesKey, active); err != nil {
		return "", err
	}
	if err := (&FirewallService{}).syncManagedSafeLocked(ctx, backend, rememberedFirewallSafetyPort()); err != nil {
		_ = saveFirewallJSON(firewallRenewalLeasesKey, active[:len(active)-1])
		TriggerFirewallSync()
		return "", fmt.Errorf("open certificate challenge firewall: %w", err)
	}
	return id, nil
}

// End removes only 3x-ui owned temporary rules via the usual reconciler.
// Existing administrator rules remain unchanged.
func RenewalFirewallEnd(id string) error {
	if id == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	firewallMu.Lock()
	defer firewallMu.Unlock()
	all, err := loadFirewallRenewalLeases()
	if err != nil {
		return err
	}
	active := activeRenewalLeases(all, time.Now())
	next := make([]firewallRenewalLease, 0, len(active))
	for _, lease := range active {
		if lease.ID != id {
			next = append(next, lease)
		}
	}
	if err := saveFirewallJSON(firewallRenewalLeasesKey, next); err != nil {
		return err
	}
	defer TriggerFirewallSync()
	backend, err := detectManagedFirewallBackend(ctx)
	if err != nil {
		return fmt.Errorf("restore certificate firewall: %w", err)
	}
	on, err := backend.enabled(ctx)
	if err != nil {
		return err
	}
	if !on {
		return nil
	}
	return (&FirewallService{}).syncManagedSafeLocked(ctx, backend, rememberedFirewallSafetyPort())
}

func WithRenewalFirewall(ctx context.Context, owner string, ports []int, operation func(context.Context) error) (err error) {
	id, err := RenewalFirewallBegin(ctx, owner, ports)
	if err != nil {
		return err
	}
	defer func() {
		if cleanupErr := RenewalFirewallEnd(id); cleanupErr != nil {
			logger.Error("certificate firewall restoration failed:", cleanupErr)
			err = errors.Join(err, cleanupErr)
		}
	}()
	return operation(ctx)
}

// CertMagic emits cert_obtaining before ACME validation, and obtained/failed
// when it completes. Separate IDs protect overlapping issuance operations.
func DomainCertificateFirewallEvents() func(context.Context, string, map[string]any) error {
	var mu sync.Mutex
	inFlight := map[string][]string{}
	return func(ctx context.Context, event string, payload map[string]any) error {
		name, _ := payload["identifier"].(string)
		if name == "" {
			return nil
		}
		switch event {
		case "cert_obtaining":
			id, err := RenewalFirewallBegin(ctx, "certmagic:"+name, []int{80, 443})
			if err != nil {
				return err
			}
			mu.Lock()
			inFlight[name] = append(inFlight[name], id)
			mu.Unlock()
		case "cert_obtained", "cert_failed":
			mu.Lock()
			ids := inFlight[name]
			if len(ids) == 0 {
				mu.Unlock()
				return nil
			}
			id := ids[len(ids)-1]
			if len(ids) == 1 {
				delete(inFlight, name)
			} else {
				inFlight[name] = ids[:len(ids)-1]
			}
			mu.Unlock()
			return RenewalFirewallEnd(id)
		}
		return nil
	}
}

func needsCertificateFirewallReconcile() bool {
	leases, err := loadFirewallRenewalLeases()
	if err == nil && len(leases) > 0 {
		return true
	}
	managed, err := loadManagedFirewallRules()
	if err != nil {
		return false
	}
	for _, rule := range managed {
		if rule.Source == "certificate-renewal" {
			return true
		}
	}
	return false
}
