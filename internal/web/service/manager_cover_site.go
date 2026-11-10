package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/mail"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

// ManagerCoverSiteInput explicitly provisions an operator-owned HTTPS decoy
// using the existing Domain Front / CertMagic implementation. Unlike a
// REALITY external camouflage target, this website is actually hosted by
// the selected node and requires control over its public DNS and ACME ports.
type ManagerCoverSiteInput struct {
	NodeID      int    `json:"nodeId"`
	Domain      string `json:"domain"`
	Email       string `json:"email"`
	AcceptTerms bool   `json:"acceptTerms"`
}

type ManagerCoverSiteResult struct {
	NodeID    int    `json:"nodeId"`
	Domain    string `json:"domain"`
	Running   bool   `json:"running"`
	State     string `json:"state"`
	ExpiresAt int64  `json:"expiresAt"`
}

// Serialize read-modify-apply of Domain Front configurations. This mutex is
// separate from managerMu so certificate issuance never blocks client jobs.
var managerCoverSiteMu sync.Mutex

// managerCoverSiteConfig keeps all unrelated domains and listeners unchanged.
// A conflicting existing domain mode must be edited via Domain Front, never
// silently overwritten by the subscription manager.
func managerCoverSiteConfig(cfg domainfront.DomainFrontConfig, req ManagerCoverSiteInput) (domainfront.DomainFrontConfig, error) {
	if !req.AcceptTerms {
		return cfg, fmt.Errorf("explicit agreement to Let's Encrypt terms is required")
	}
	domainRaw := strings.TrimSpace(req.Domain)
	if domainRaw == "" || strings.ContainsAny(domainRaw, "/:@?#\\") {
		return cfg, fmt.Errorf("enter a DNS hostname, not a URL")
	}
	domain, err := managerDomain(domainRaw)
	if err != nil {
		return cfg, err
	}
	email := strings.TrimSpace(req.Email)
	parsedEmail, err := mail.ParseAddress(email)
	if err != nil || parsedEmail.Address != email || len(email) > 254 {
		return cfg, fmt.Errorf("a valid ACME contact email is required")
	}
	// Older nodes without Domain Front must be upgraded before provisioning.
	// Never overwrite an unknown node config with zero-value listener settings.
	if cfg.HTTPSPort < 1 || cfg.HTTPPort < 1 || cfg.Listen == "" {
		return cfg, fmt.Errorf("Domain Front configuration unavailable; upgrade or configure the selected node first")
	}
	cfg.Domains = append([]domainfront.DomainFrontDomain(nil), cfg.Domains...)
	found := false
	for i := range cfg.Domains {
		d := &cfg.Domains[i]
		if !strings.EqualFold(strings.TrimSuffix(d.Name, "."), domain) {
			continue
		}
		if d.Mode != "sni_only" || d.Panel || d.Origin != "" || len(d.InboundIDs) != 0 {
			return cfg, fmt.Errorf("domain %s already has another purpose; edit it under Domains and TLS", domain)
		}
		if d.TLS != "auto" && d.TLS != "certmagic" {
			return cfg, fmt.Errorf("domain %s uses a manually managed certificate; edit it under Domains and TLS", domain)
		}
		d.Name, d.Enabled, d.TLS = domain, true, "certmagic"
		found = true
		break
	}
	if !found {
		if len(cfg.Domains) >= 100 {
			return cfg, fmt.Errorf("domain gateway limit of 100 sites reached")
		}
		cfg.Domains = append(cfg.Domains, domainfront.DomainFrontDomain{
			Name: domain, Mode: "sni_only", TLS: "certmagic", Enabled: true, InboundIDs: []int{},
		})
	}
	cfg.Enabled, cfg.AcceptTerms, cfg.Email = true, true, email
	return cfg, nil
}

// ProvisionCoverSite is opt-in only. If a node runs a conflicting HTTPS
// listener or DNS is not delegated, the existing Domain Front operation fails
// safely with its built-in rollback; the panel never fabricates a certificate.
func (s *ManagerService) ProvisionCoverSite(ctx context.Context, owner int, req ManagerCoverSiteInput) (ManagerCoverSiteResult, error) {
	result := ManagerCoverSiteResult{NodeID: req.NodeID}
	if owner < 1 || req.NodeID < 0 {
		return result, fmt.Errorf("invalid node or owner")
	}
	managerCoverSiteMu.Lock()
	defer managerCoverSiteMu.Unlock()
	// Do not hold the manager operation mutex during an ACME operation:
	// issuance may take time and must not block client creation/rotation.
	var current domainfront.DomainFrontStatus
	var remote *runtime.Remote
	if req.NodeID == 0 {
		status, err := DomainFrontStatus()
		if err != nil {
			return result, err
		}
		current = status
	} else {
		var node model.Node
		if err := database.GetDB().First(&node, req.NodeID).Error; err != nil {
			return result, err
		}
		if !node.Enable || node.Status != "online" {
			return result, fmt.Errorf("selected node must be enabled and online")
		}
		manager := runtime.GetManager()
		if manager == nil {
			return result, fmt.Errorf("node manager is unavailable")
		}
		var err error
		remote, err = manager.RemoteFor(&node)
		if err != nil {
			return result, err
		}
		response, err := remote.ReadDomainFrontStatus(ctx)
		if err != nil {
			return result, fmt.Errorf("read node TLS settings: %w", err)
		}
		if err := json.Unmarshal(response, &current); err != nil {
			return result, fmt.Errorf("decode node TLS settings: %w", err)
		}
	}
	cfg, err := managerCoverSiteConfig(current.Config, req)
	if err != nil {
		return result, err
	}
	var updated domainfront.DomainFrontStatus
	if remote == nil {
		updated, err = ApplyDomainFront(ctx, cfg)
	} else {
		applyCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
		defer cancel()
		var raw json.RawMessage
		raw, err = remote.ApplyDomainFrontConfig(applyCtx, cfg)
		if err == nil {
			err = json.Unmarshal(raw, &updated)
		}
	}
	if err != nil {
		return result, err
	}
	result.Domain = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(req.Domain)), ".")
	result.Running = updated.Running
	result.State = "pending"
	for _, cert := range updated.Certificates {
		if cert.Name == result.Domain {
			result.State, result.ExpiresAt = cert.State, cert.ExpiresAt
			break
		}
	}
	return result, nil
}
