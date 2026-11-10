package service

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
	"github.com/SawaMEN/3x-ui/v3/internal/web/runtime"
)

const maxIndependentObservers = 3
const maxObservedDomains = 3

// augmentDomainPreflightWithNodeEvidence is a bounded diagnostic, not a
// prerequisite for obtaining a new certificate. ACME performs the final
// public verification; these independent observations describe the currently
// running gateway, never a not-yet-applied draft.
func augmentDomainPreflightWithNodeEvidence(ctx context.Context, cfg domainfront.DomainFrontConfig, report *domainPreflightReport) {
	if !cfg.Enabled {
		return
	}
	// The UI uses a 15-second deadline. Together with the six-second
	// built-in DNS preflight, node checks must finish within that budget.
	ctx, cancelAll := context.WithTimeout(ctx, 7*time.Second)
	defer cancelAll()
	augmentNodeCertificateEvidence(ctx, cfg, report)
	for _, check := range report.Checks {
		if check.Code != "dns_resolved" || len(check.Addresses) == 0 {
			continue
		}
		bound := net.ParseIP(cfg.Listen)
		if bound == nil || netsafe.IsBlockedIP(bound) {
			continue // 0.0.0.0 / NAT: no authoritative public IP is known
		}
		match := false
		for _, address := range check.Addresses {
			if ip := net.ParseIP(address); ip != nil && ip.Equal(bound) {
				match = true
				break
			}
		}
		code, severity, message := "dns_ip_matches_listener", "info", "DNS includes the configured public gateway listener address."
		if !match {
			code, severity, message = "dns_ip_listener_mismatch", "warning",
				"DNS does not include the configured public listener IP. A reverse proxy or NAT may explain the difference."
		}
		report.Checks = append(report.Checks, domainPreflightCheck{
			Code: code, Severity: severity, Domain: check.Domain, Message: message,
		})
	}
	// Never request HTTP-01 checks for wildcard/DNS-01, manual-only or
	// foreign references. An existing certificate does not imply this
	// observation is needed or that the draft will be reachable.
	var names []string
	for _, d := range cfg.Domains {
		if !d.Enabled || d.Mode == "shared_sni" || strings.HasPrefix(d.Name, "*.") ||
			(d.TLS != "auto" && d.TLS != "certmagic") || cfg.DNS01Provider != "" {
			continue
		}
		names = append(names, d.Name)
		if len(names) >= maxObservedDomains {
			break
		}
	}
	if len(names) == 0 {
		return
	}
	if cfg.HTTPPort != 80 {
		return // the existing preflight already warns about port 80
	}
	manager := runtime.GetManager()
	db := database.GetDB()
	if manager == nil || db == nil {
		return
	}
	var nodes []model.Node
	if err := db.Where("enable = ?", true).Limit(maxIndependentObservers).Find(&nodes).Error; err != nil || len(nodes) == 0 {
		report.Checks = append(report.Checks, domainPreflightCheck{
			Code: "http01_external_observer_missing", Severity: "info",
			Message: "No connected independent node is available to verify public HTTP-01. DNS and local port checks alone do not prove reachability.",
		})
		return
	}
	for _, name := range names {
		// Only the currently running gateway can answer the random proof.
		// A new hostname is reported as not yet verifiable, never as passed.
		token, cleanup, err := managedDomainFront().StartHTTP01Probe(name)
		draftProof := false
		if err != nil {
			// Prove a new draft without applying the gateway: bind TCP 80
			// only if it is truly free, and serve only a random challenge.
			token, cleanup, err = domainfront.StartDraftHTTP01Probe(ctx, cfg, name)
			draftProof = err == nil
		}
		if err != nil {
			report.Checks = append(report.Checks, domainPreflightCheck{
				Code: "http01_unavailable", Severity: "info", Domain: name,
				Message: "External HTTP-01 could not be proved without taking over an existing port 80 listener. Verify NAT, firewall and routing on the target host.",
			})
			continue
		}
		probeCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
		type observation struct {
			nodeName string
			verified bool
			detail string
		}
		results := make(chan observation, len(nodes))
		var wg sync.WaitGroup
		for _, node := range nodes {
			if node.Id <= 0 {
				continue
			}
			wg.Add(1)
			go func(node model.Node) {
				defer wg.Done()
				remote, err := manager.RemoteFor(&node)
				if err != nil {
					results <- observation{nodeName: node.Name, detail: "unavailable"}
					return
				}
				seen, err := remote.ProbePublicHTTP01(probeCtx, name, token)
				if err != nil {
					results <- observation{nodeName: node.Name, detail: "unsupported or unreachable"}
					return
				}
				results <- observation{nodeName: node.Name, verified: seen.Verified, detail: seen.Detail}
			}(node)
		}
		wg.Wait()
		close(results)
		cancel()
		cleanup()
		successes, failures := 0, 0
		for result := range results {
			if result.verified {
				successes++
			} else {
				failures++
			}
		}
		code, severity := "http01_external_unverified", "warning"
		msg := fmt.Sprintf("The active HTTP-01 gateway was not independently verified (%d/%d observers). Review public TCP 80, NAT and firewall rules.", successes, successes+failures)
		if successes > 0 {
			code, severity = "http01_external_verified", "info"
			if draftProof {
				msg = fmt.Sprintf("A temporary HTTP-01 listener verified this unsaved hostname from %d independent node(s); no gateway configuration was applied.", successes)
			} else {
				msg = fmt.Sprintf("The active gateway returned a matching HTTP-01 proof from %d independent node(s). Changes to the draft may still affect reachability.", successes)
			}
		}
		report.Checks = append(report.Checks, domainPreflightCheck{
			Code: code, Severity: severity, Domain: name, Message: msg,
		})
	}
}

// Remote SNI certificate paths never cross the node boundary. Inspect the
// authenticated metadata only, and compare a configured public node address
// with DNS as advisory evidence (NAT/CDN fronting may be intentional).
func augmentNodeCertificateEvidence(ctx context.Context, cfg domainfront.DomainFrontConfig, report *domainPreflightReport) {
	var shared []string
	for _, d := range cfg.Domains {
		if d.Enabled && d.Mode == "shared_sni" {
			shared = append(shared, d.Name)
		}
		if len(shared) >= 8 {
			break
		}
	}
	if len(shared) == 0 {
		return
	}
	certs := readRemoteCertificateInventories(ctx)
	db := database.GetDB()
	nodeAddresses := make(map[int]string)
	if db != nil {
		var nodes []model.Node
		if err := db.Select("id", "address").Where("enable = ?", true).Limit(64).Find(&nodes).Error; err == nil {
			for _, node := range nodes {
				nodeAddresses[node.Id] = node.Address
			}
		}
	}
	for _, name := range shared {
		certificateFound := false
		for _, item := range certs {
			if item.NodeID <= 0 || item.Name != name {
				continue
			}
			certificateFound = true
			code, severity := "node_certificate_valid", "info"
			message := fmt.Sprintf("Node #%d owns the matching certificate; its private key remains on that node.", item.NodeID)
			if item.State == "expired" || item.State == "error" || item.DaysLeft < 0 {
				code, severity = "node_certificate_invalid", "warning"
				message = fmt.Sprintf("Node #%d reports a certificate that is expired or unhealthy.", item.NodeID)
			} else if item.State != "valid" && item.State != "renewal_due" {
				code = "node_certificate_unknown"
				message = fmt.Sprintf("Node #%d has not confirmed a valid certificate for this SNI.", item.NodeID)
			}
			report.Checks = append(report.Checks, domainPreflightCheck{
				Code: code, Severity: severity, Domain: name, Message: message,
			})
			// Only compare literal global IPs. A host name, private node
			// address, NAT or a CDN does not provide a proven public mapping.
			nodeIP := net.ParseIP(nodeAddresses[item.NodeID])
			if nodeIP == nil || netsafe.IsBlockedIP(nodeIP) {
				continue
			}
			dnsCtx, stop := context.WithTimeout(ctx, 2*time.Second)
			addrs, err := net.DefaultResolver.LookupIPAddr(dnsCtx, name)
			stop()
			if err != nil {
				continue
			}
			matched := false
			for _, entry := range addrs {
				if entry.IP.Equal(nodeIP) {
					matched = true
					break
				}
			}
			if !matched {
				report.Checks = append(report.Checks, domainPreflightCheck{
					Code: "node_dns_address_mismatch", Severity: "warning", Domain: name,
					Message: fmt.Sprintf("DNS does not include node #%d's configured public IP; verify CDN, NAT or proxy mapping.", item.NodeID),
				})
			} else {
				report.Checks = append(report.Checks, domainPreflightCheck{
					Code: "node_dns_address_match", Severity: "info", Domain: name,
					Message: fmt.Sprintf("DNS includes node #%d's configured public IP.", item.NodeID),
				})
			}
		}
		if !certificateFound {
			report.Checks = append(report.Checks, domainPreflightCheck{
				Code: "node_certificate_unverified", Severity: "info", Domain: name,
				Message: "No connected node supplied certificate evidence for this shared SNI. Verify on the owning node.",
			})
		}
	}
}
