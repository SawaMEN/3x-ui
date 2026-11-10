package service

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

// A preflight is advisory and read-only: it neither binds a long-lived
// listener, requests an ACME certificate, nor applies a gateway config.
type domainPreflightCheck struct {
	Code      string   `json:"code"`
	Severity  string   `json:"severity"`
	Domain    string   `json:"domain,omitempty"`
	Message   string   `json:"message"`
	Addresses []string `json:"addresses,omitempty"`
}

type domainPreflightReport struct {
	Ready     bool                   `json:"ready"`
	CheckedAt int64                  `json:"checkedAt"`
	Checks    []domainPreflightCheck `json:"checks"`
}

type preflightDeps struct {
	lookup      func(context.Context, string) ([]net.IPAddr, error)
	probePort   func(context.Context, string, int) error
	telemtCover string
}

// PreflightDomainFront checks the supplied unsaved draft without changing it.
// DNS checks are intentionally advisory: a brand-new hostname may not be
// published yet, and the panel cannot prove external HTTP-01 reachability.
func PreflightDomainFront(ctx context.Context, cfg domainfront.DomainFrontConfig) (domainPreflightReport, error) {
	s := &SettingService{}
	files, _, err := s.domainCertificateFiles(cfg)
	if err != nil {
		return domainPreflightReport{}, err
	}
	old, err := s.GetDomainFrontConfig()
	if err != nil {
		return domainPreflightReport{}, err
	}
	status, err := DomainFrontStatus()
	if err != nil {
		return domainPreflightReport{}, err
	}
	var telemtCover string
	if state, stateErr := readTelemtWebState(); stateErr == nil && state.Enabled && telemtWebFrontendActive(state) {
		telemtCover = state.Domain
	}
	report := runDomainPreflight(ctx, cfg, files, old, status.Running, preflightDeps{
		lookup:      net.DefaultResolver.LookupIPAddr,
		probePort:   probeDomainPort,
		telemtCover: telemtCover,
	})
	if cfg.Enabled && cfg.DNS01Provider != "" {
		credential, tokenErr := DomainDNSCredentialStateFor(cfg.DNS01Provider)
		if tokenErr != nil || !credential.Configured {
			report.Ready = false
			report.Checks = append(report.Checks, domainPreflightCheck{
				Code: "dns01_credentials_missing", Severity: "error",
				Message: "Selected DNS-01 provider requires a scoped API token configured on this server.",
			})
		}
	}
	augmentDomainPreflightWithNodeEvidence(ctx, cfg, &report)
	return report, nil
}

func probeDomainPort(ctx context.Context, listen string, port int) error {
	var config net.ListenConfig
	listener, err := config.Listen(ctx, "tcp", net.JoinHostPort(listen, strconv.Itoa(port)))
	if err != nil {
		return err
	}
	return listener.Close()
}

func runDomainPreflight(
	ctx context.Context,
	cfg domainfront.DomainFrontConfig,
	files map[string]domainfront.CertificateFiles,
	old domainfront.DomainFrontConfig,
	previousRunning bool,
	deps preflightDeps,
) domainPreflightReport {
	result := domainPreflightReport{Ready: true, CheckedAt: time.Now().Unix(), Checks: []domainPreflightCheck{}}
	add := func(code, severity, name, message string, addresses ...string) {
		if severity == "error" {
			result.Ready = false
		}
		result.Checks = append(result.Checks, domainPreflightCheck{
			Code: code, Severity: severity, Domain: name, Message: message, Addresses: addresses,
		})
	}
	// Validation normalizes a copy; do not change the UI's submitted draft.
	cfg.Domains = append([]domainfront.DomainFrontDomain(nil), cfg.Domains...)
	for _, domain := range cfg.Domains {
		if domain.Panel {
			add("panel_forbidden", "error", domain.Name, "Gateway domains cannot expose the admin panel.")
		}
	}
	if err := cfg.ValidateWithCertificates(files); err != nil {
		add("invalid_config", "error", "", err.Error())
		return result
	}
	if !cfg.Enabled {
		add("gateway_disabled", "info", "", "The gateway is disabled; no listeners or domain probes are required.")
		return result
	}
	local := make([]domainfront.DomainFrontDomain, 0, len(cfg.Domains))
	for _, domain := range cfg.Domains {
		if !domain.Enabled {
			continue
		}
		if domain.Mode == "shared_sni" {
			add("remote_sni", "info", domain.Name, "This SNI is managed on another node; its private key stays on that node.")
			continue
		}
		local = append(local, domain)
	}
	if len(local) == 0 {
		add("no_local_domains", "info", "", "There are no enabled local gateway domains.")
		return result
	}
	if deps.telemtCover != "" && cfg.HTTPSPort == 443 {
		add("telemt_port_conflict", "error", "", fmt.Sprintf("Telemt WEB owns TCP 443 for %s. Reuse its SNI or select a different HTTPS gateway port.", deps.telemtCover))
	}
	if deps.probePort != nil {
		for _, entry := range []struct {
			name  string
			port  int
			owned bool
		}{
			{"HTTP", cfg.HTTPPort, previousRunning && old.Enabled && old.Listen == cfg.Listen && old.HTTPPort == cfg.HTTPPort},
			{"HTTPS", cfg.HTTPSPort, previousRunning && old.Enabled && old.Listen == cfg.Listen && old.HTTPSPort == cfg.HTTPSPort},
		} {
			if entry.owned {
				add("port_owned", "info", "", fmt.Sprintf("%s TCP %d is already used by the running domain gateway.", entry.name, entry.port))
			} else if err := deps.probePort(ctx, cfg.Listen, entry.port); err != nil {
				// Existing panel/subscription listeners may be intentionally
				// shared with the gateway. Do not mark this as a hard error.
				add("port_unavailable", "warning", "", fmt.Sprintf("%s TCP %d could not be bound (%s); check for a listener conflict or compatible shared listener.", entry.name, entry.port, err))
			}
		}
	}
	if cfg.DNS01Provider == "" && cfg.HTTPPort != 80 {
		for _, domain := range local {
			if domain.TLS == "certmagic" || domain.TLS == "auto" && files[domain.Name].CertFile == "" {
				add("http01_port", "warning", "", "Automatic HTTP-01 validation normally requires public TCP 80; verify external NAT or use another supported challenge method.")
				break
			}
		}
	}
	for _, domain := range local {
		if domain.TLS != "manual" {
			continue
		}
		pair, err := tls.LoadX509KeyPair(domain.CertFile, domain.KeyFile)
		if err != nil || len(pair.Certificate) == 0 {
			add("manual_certificate_unreadable", "error", domain.Name, "The manual TLS certificate or private key cannot be loaded.")
			continue
		}
		leaf, err := x509.ParseCertificate(pair.Certificate[0])
		if err != nil {
			add("manual_certificate_invalid", "error", domain.Name, "The manual TLS certificate cannot be parsed.")
			continue
		}
		if leaf.VerifyHostname(domain.Name) != nil {
			add("manual_certificate_hostname", "error", domain.Name, "The manual TLS certificate does not cover this hostname.")
		}
		if time.Now().Before(leaf.NotBefore) || !time.Now().Before(leaf.NotAfter) {
			add("manual_certificate_expired", "error", domain.Name, "The manual TLS certificate is not currently valid.")
		} else if time.Until(leaf.NotAfter) < 30*24*time.Hour {
			add("manual_certificate_expiring", "warning", domain.Name, "The manual TLS certificate expires within 30 days.")
		}
	}
	if deps.lookup == nil {
		return result
	}
	// Bound request duration and concurrency; DNS is never used to dial a
	// resolved IP from this endpoint, avoiding an SSRF network primitive.
	checkCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	type dnsResult struct {
		ips []net.IPAddr
		err error
	}
	answers := make([]dnsResult, len(local))
	jobs := make(chan int)
	var workers sync.WaitGroup
	for i := 0; i < 4 && i < len(local); i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				answers[index].ips, answers[index].err = deps.lookup(checkCtx, local[index].Name)
			}
		}()
	}
dispatch:
	for index := range local {
		if strings.HasPrefix(local[index].Name, "*.") {
			continue // wildcard DNS A record is not meaningful
		}
		select {
		case jobs <- index:
		case <-checkCtx.Done():
			break dispatch
		}
	}
	close(jobs)
	workers.Wait()
	for i, domain := range local {
		if strings.HasPrefix(domain.Name, "*.") {
			add("wildcard_dns01", "info", domain.Name, "DNS-01 validates the wildcard via a temporary TXT record. Individual wildcard subdomain A/AAAA records cannot be established from this draft.")
			continue
		}
		answer := answers[i]
		if answer.err != nil {
			add("dns_lookup_failed", "warning", domain.Name, fmt.Sprintf("DNS lookup failed: %s", answer.err))
			continue
		}
		if len(answer.ips) == 0 {
			add("dns_missing", "warning", domain.Name, "No DNS A/AAAA records were found.")
			continue
		}
		addresses := make([]string, 0, len(answer.ips))
		hasPublic := false
		for _, entry := range answer.ips {
			addresses = append(addresses, entry.IP.String())
			if entry.IP.IsGlobalUnicast() && !entry.IP.IsPrivate() && !entry.IP.IsLoopback() {
				hasPublic = true
			}
		}
		if !hasPublic {
			add("dns_private", "warning", domain.Name, "DNS resolves only to non-public addresses.", addresses...)
		} else {
			add("dns_resolved", "info", domain.Name, "DNS A/AAAA records were resolved. This does not prove the server IP, HTTP-01 reachability or TLS readiness.", addresses...)
		}
	}
	if errors.Is(checkCtx.Err(), context.DeadlineExceeded) {
		add("dns_timeout", "warning", "", "Some DNS lookups exceeded the diagnostic deadline.")
	}
	return result
}
