package service

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
)

func testPreflightConfig() domainfront.DomainFrontConfig {
	cfg := domainfront.DefaultConfig()
	cfg.Enabled = true
	cfg.AcceptTerms = true
	cfg.Domains = []domainfront.DomainFrontDomain{{
		Name: "sub.example.org", Mode: "sub_link_only", Enabled: true, TLS: "auto",
	}}
	return cfg
}

func testPreflightDeps() preflightDeps {
	return preflightDeps{
		lookup: func(_ context.Context, _ string) ([]net.IPAddr, error) {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		},
		probePort: func(string, int) error { return nil },
	}
}

func issueCount(report domainPreflightReport, code, severity string) int {
	count := 0
	for _, check := range report.Checks {
		if check.Code == code && check.Severity == severity {
			count++
		}
	}
	return count
}

func TestDomainPreflightReadOnlyAndDNS(t *testing.T) {
	cfg := testPreflightConfig()
	report := runDomainPreflight(context.Background(), cfg, nil, domainfront.DefaultConfig(), false, testPreflightDeps())
	if !report.Ready || issueCount(report, "dns_resolved", "info") != 1 {
		t.Fatalf("unexpected preflight: %+v", report)
	}
	if cfg.Domains[0].Name != "sub.example.org" || len(cfg.Domains[0].InboundIDs) != 0 {
		t.Fatalf("preflight mutated its input: %+v", cfg)
	}
}

func TestDomainPreflightInvalidConfigBlocksAndSkipsNetwork(t *testing.T) {
	cfg := testPreflightConfig()
	cfg.Domains[0].Name = "127.0.0.1"
	probed := false
	deps := testPreflightDeps()
	deps.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		probed = true
		return nil, nil
	}
	report := runDomainPreflight(context.Background(), cfg, nil, domainfront.DefaultConfig(), false, deps)
	if report.Ready || issueCount(report, "invalid_config", "error") != 1 || probed {
		t.Fatalf("invalid config performed DNS or was not rejected: %+v", report)
	}
}

func TestDomainPreflightConflictingPortAndRemoteSNI(t *testing.T) {
	cfg := testPreflightConfig()
	deps := testPreflightDeps()
	deps.probePort = func(_ string, port int) error {
		if port == 443 { return errors.New("address in use") }
		return nil
	}
	report := runDomainPreflight(context.Background(), cfg, nil, domainfront.DefaultConfig(), false, deps)
	if !report.Ready || issueCount(report, "port_unavailable", "warning") != 1 {
		t.Fatalf("busy port must produce a warning: %+v", report)
	}
	deps.telemtCover = "cover.example.org"
	report = runDomainPreflight(context.Background(), cfg, nil, domainfront.DefaultConfig(), false, deps)
	if report.Ready || issueCount(report, "telemt_port_conflict", "error") != 1 {
		t.Fatalf("Telemt ownership must block conflicting gateway: %+v", report)
	}
	cfg.Domains = []domainfront.DomainFrontDomain{{Name: "remote.example.org", Mode: "shared_sni", Enabled: true, TLS: "auto"}}
	report = runDomainPreflight(context.Background(), cfg, nil, domainfront.DefaultConfig(), false, deps)
	if !report.Ready || issueCount(report, "remote_sni", "info") != 1 || issueCount(report, "telemt_port_conflict", "error") != 0 {
		t.Fatalf("remote SNI cannot take local port ownership: %+v", report)
	}
}

func TestDomainPreflightDNSUnavailableIsAdvisory(t *testing.T) {
	cfg := testPreflightConfig()
	deps := testPreflightDeps()
	deps.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return nil, errors.New("NXDOMAIN")
	}
	report := runDomainPreflight(context.Background(), cfg, nil, domainfront.DefaultConfig(), false, deps)
	if !report.Ready || issueCount(report, "dns_lookup_failed", "warning") != 1 || !strings.Contains(report.Checks[len(report.Checks)-1].Message, "NXDOMAIN") {
		t.Fatalf("DNS unavailable should not be a hard blocker: %+v", report)
	}
}
