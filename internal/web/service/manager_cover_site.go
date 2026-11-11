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
	SiteCatalog string `json:"siteCatalog"`
	SiteTemplate string `json:"siteTemplate"`
	IssueOnMain bool `json:"issueOnMain"`
}

type ManagerCoverSiteResult struct {
	NodeID    int    `json:"nodeId"`
	Domain    string `json:"domain"`
	Running   bool   `json:"running"`
	State     string `json:"state"`
	ExpiresAt int64  `json:"expiresAt"`
	MainState string `json:"mainState,omitempty"`
	MainCertFile string `json:"mainCertFile,omitempty"`
	MainKeyFile string `json:"mainKeyFile,omitempty"`
	MainError string `json:"mainError,omitempty"`
}

// Serialize read-modify-apply of Domain Front configurations. This mutex is
// separate from managerMu so certificate issuance never blocks client jobs.
var managerCoverSiteMu sync.Mutex

// managerCoverSiteConfig keeps all unrelated domains and listeners unchanged.
// A conflicting existing domain mode must be edited via Domain Front, never
// silently overwritten by the subscription manager.
func managerCoverSiteConfig(cfg domainfront.DomainFrontConfig, req ManagerCoverSiteInput) (domainfront.DomainFrontConfig, error) {
	if (req.SiteCatalog == "") != (req.SiteTemplate == "") {
		return cfg, fmt.Errorf("select both a cover site catalog and template")
	}
	if req.SiteCatalog != "" {
		if err := validateTelemtWebSite(&TelemtWebSiteOptions{Mode: "catalog", Catalog: req.SiteCatalog, Template: req.SiteTemplate}); err != nil {
			return cfg, err
		}
	}
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
		d.SiteCatalog, d.SiteTemplate = req.SiteCatalog, req.SiteTemplate
		found = true
		break
	}
	if !found {
		if len(cfg.Domains) >= 100 {
			return cfg, fmt.Errorf("domain gateway limit of 100 sites reached")
		}
		cfg.Domains = append(cfg.Domains, domainfront.DomainFrontDomain{
			Name: domain, Mode: "sni_only", TLS: "certmagic", Enabled: true, InboundIDs: []int{},
			SiteCatalog: req.SiteCatalog, SiteTemplate: req.SiteTemplate,
		})
	}
	cfg.Enabled, cfg.AcceptTerms, cfg.Email = true, true, email
	return cfg, nil
}

// managerMainCertificateConfig sets up main-node DNS-01 issuance without a
// second public website. A foreign shared_sni reference can be explicitly
// upgraded, but real sites and manual TLS settings must never be overwritten.
func managerMainCertificateConfig(cfg domainfront.DomainFrontConfig, domain, email string) (domainfront.DomainFrontConfig, error) {
	if cfg.DNS01Provider == "" {
		return cfg, fmt.Errorf("configure Cloudflare or DigitalOcean DNS-01 on the main node first")
	}
	if cfg.DNS01Zone == "" || (domain != cfg.DNS01Zone && !strings.HasSuffix(domain, "."+cfg.DNS01Zone)) {
		return cfg, fmt.Errorf("the main node DNS-01 zone does not cover %s", domain)
	}
	cfg.Domains = append([]domainfront.DomainFrontDomain(nil), cfg.Domains...)
	for i := range cfg.Domains {
		d := &cfg.Domains[i]
		if d.Name != domain {
			continue
		}
		if d.Mode != "cert_only" && d.Mode != "shared_sni" {
			return cfg, fmt.Errorf("%s already has another purpose on the main node", domain)
		}
		*d = domainfront.DomainFrontDomain{Name: domain, Mode: "cert_only", TLS: "certmagic", Enabled: true, InboundIDs: []int{}}
		if cfg.Email == "" { cfg.Email = email }
		cfg.Enabled, cfg.AcceptTerms = true, true
		return cfg, nil
	}
	if len(cfg.Domains) >= 100 {
		return cfg, fmt.Errorf("domain gateway limit of 100 sites reached")
	}
	cfg.Domains = append(cfg.Domains, domainfront.DomainFrontDomain{Name: domain, Mode: "cert_only", TLS: "certmagic", Enabled: true, InboundIDs: []int{}})
	if cfg.Email == "" { cfg.Email = email }
	cfg.Enabled, cfg.AcceptTerms = true, true
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
	// Plan the local DNS-01 operation before writing anything to the remote
	// node. Existing locally verified certificates require no new ACME order.
	var mainPlan *domainfront.DomainFrontConfig
	if req.NodeID > 0 && req.IssueOnMain {
		mainCerts, err := ManagedSNIDomains(ctx, false)
		if err != nil {
			return result, err
		}
		name, err := managerDomain(strings.TrimSpace(req.Domain))
		if err != nil {
			return result, err
		}
		for _, cert := range mainCerts {
			if cert.Name == name && cert.CertFile != "" && cert.KeyFile != "" {
				result.MainState, result.MainCertFile, result.MainKeyFile = "valid", cert.CertFile, cert.KeyFile
				break
			}
		}
		if result.MainState == "" {
			mainStatus, err := DomainFrontStatus()
			if err != nil {
				return result, err
			}
			plan, err := managerMainCertificateConfig(mainStatus.Config, name, strings.TrimSpace(req.Email))
			if err != nil {
				return result, err
			}
			credential, err := DomainDNSCredentialStateFor(plan.DNS01Provider)
			if err != nil || !credential.Configured {
				return result, fmt.Errorf("main node DNS-01 credentials are not configured")
			}
			mainPlan = &plan
		}
	}
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
	if mainPlan != nil {
		local, localErr := ApplyDomainFront(ctx, *mainPlan)
		if localErr != nil {
			// The remote cover site was created. Return its success plus a
			// separate local failure to avoid misleading users.
			result.MainError = localErr.Error()
		} else {
			result.MainState = "pending"
			for _, cert := range local.Certificates {
				if cert.Name == result.Domain {
					result.MainState, result.MainCertFile, result.MainKeyFile = cert.State, cert.CertFile, cert.KeyFile
					break
				}
			}
		}
	}
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
