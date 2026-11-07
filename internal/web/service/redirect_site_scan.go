package service

import (
	"context"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

const (
	redirectMaxSNICandidates = 32
	redirectMaxHTTPFinalists = 16
)

// redirectSNITargets reuses the panel's REALITY/SNI search source and its
// previously verified, stable candidates. Domain names only: a redirected
// website must have a valid HTTPS hostname, not a certificate discovered on
// an unrelated IP, or a custom TCP/TLS port.
func redirectSNITargets(configured []string, history []SNIJobSnapshot) []string {
	out := make([]string, 0, redirectMaxSNICandidates)
	seen := make(map[string]bool)
	add := func(value string) {
		if len(out) >= redirectMaxSNICandidates {
			return
		}
		host, port, err := splitRealityTarget(value)
		if err != nil || port != 443 || net.ParseIP(host) != nil ||
			!strings.Contains(host, ".") || strings.HasPrefix(host, "*.") {
			return
		}
		if _, err := netsafe.NormalizeHost(host); err != nil || seen[host] {
			return
		}
		seen[host] = true
		out = append(out, net.JoinHostPort(host, "443"))
	}

	for _, scan := range history {
		if scan.State != "done" {
			continue
		}
		for _, row := range scan.Results {
			if row == nil || !row.CertValid || row.PrivateTarget || row.Port != 443 ||
				row.Attempts < 5 || row.Successes < 4 ||
				(row.DNSMatchesIP != nil && !*row.DNSMatchesIP) {
				continue
			}
			add(row.Host)
			if len(out) >= 8 {
				break
			}
		}
		if len(out) >= 8 {
			break
		}
	}

	// Spread a bounded sample across the operator's existing SNI catalog
	// instead of repeatedly scanning only the first group of 800 entries.
	// Small custom lists are used completely, subject to the cap.
	remaining := redirectMaxSNICandidates - len(out)
	if remaining <= 0 || len(configured) == 0 {
		return out
	}
	if len(configured) <= remaining {
		for _, candidate := range configured {
			add(candidate)
		}
		return out
	}
	// Sample across the full configured catalog (including the 800-entry
	// default); fill invalid/duplicate gaps in a second bounded pass.
	for i := 0; i < remaining && len(out) < redirectMaxSNICandidates; i++ {
		add(configured[i*len(configured)/remaining])
	}
	for _, candidate := range configured {
		if len(out) >= redirectMaxSNICandidates { break }
		add(candidate)
	}
	return out
}

func trustedRedirectHistory(history []SNIJobSnapshot) map[string]bool {
	stable := map[string]bool{}
	// History is newest-first. Use only recently measured, stable HTTPS
	// candidates, never historical DNS/IP bindings as actual dial targets.
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	for _, scan := range history {
		for _, row := range scan.Results {
			if row == nil || !row.CertValid || row.PrivateTarget || row.Port != 443 ||
				row.Attempts < 5 || row.Successes < 4 ||
				(row.DNSMatchesIP != nil && !*row.DNSMatchesIP) {
				continue
			}
			checked, err := time.Parse(time.RFC3339, row.CheckedAt)
			if err != nil || checked.Before(cutoff) {
				continue
			}
			stable[strings.ToLower(row.Host)] = true
		}
	}
	return stable
}

// ScanRedirectSites uses the shared rate-limited SNI/TLS scanner for the
// first-stage certificate check, then verifies real HTTPS page behavior.
// This is deliberately not a full 800-host scan on every button press.
func ScanRedirectSites(ctx context.Context) (domainfront.DecoySiteScan, error) {
	svc := &ServerService{}
	history, _ := SNIScanHistory()
	targets := redirectSNITargets(svc.realityScanCandidateTokens(), history)
	stable := trustedRedirectHistory(history)
	// Allow time for the site's actual HTTP/path check after the TLS stage.
	tlsCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	results, err := svc.ScanRealityTargetsContext(tlsCtx, strings.Join(targets, ","))
	cancel()
	if err != nil && ctx.Err() != nil {
		return domainfront.DecoySiteScan{}, ctx.Err()
	}
	candidates := make([]domainfront.DecoyCandidate, 0, redirectMaxHTTPFinalists)
	for _, row := range results {
		if row == nil || !row.CertValid || row.PrivateTarget || row.Port != 443 ||
			net.ParseIP(row.Host) != nil || row.IP == "" || netsafe.IsBlockedIP(net.ParseIP(row.IP)) {
			continue
		}
		candidates = append(candidates, domainfront.DecoyCandidate{
			URL: "https://" + row.Host + "/",
			SNILatencyMS: int64(row.LatencyMs),
			TLS13: row.TLS13,
			HTTP2: row.H2,
			SNIStable: stable[strings.ToLower(row.Host)],
		})
	}
	// Prefer TLS-compatible candidates, but do not require REALITY-specific
	// X25519 or HTTP/2 behavior: this is a website redirect, not REALITY.
	slices.SortStableFunc(candidates, func(a, b domainfront.DecoyCandidate) int {
		if a.SNIStable != b.SNIStable {
			if a.SNIStable { return -1 }
			return 1
		}
		if a.SNILatencyMS < b.SNILatencyMS { return -1 }
		if a.SNILatencyMS > b.SNILatencyMS { return 1 }
		return strings.Compare(a.URL, b.URL)
	})
	if len(candidates) > redirectMaxHTTPFinalists {
		candidates = candidates[:redirectMaxHTTPFinalists]
	}
	return domainfront.ScanDecoySites(ctx, candidates), nil
}
