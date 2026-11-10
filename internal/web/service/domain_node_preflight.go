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
		if err != nil {
			report.Checks = append(report.Checks, domainPreflightCheck{
				Code: "http01_not_active", Severity: "info", Domain: name,
				Message: "External HTTP-01 cannot be proved until this gateway hostname is running on port 80.",
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
			msg = fmt.Sprintf("The active gateway returned a matching temporary HTTP-01 proof from %d independent node(s). This does not verify an unsaved configuration.", successes)
		}
		report.Checks = append(report.Checks, domainPreflightCheck{
			Code: code, Severity: severity, Domain: name, Message: msg,
		})
	}
}
