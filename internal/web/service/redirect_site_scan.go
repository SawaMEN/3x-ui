package service

import (
	"context"
	"net"
	"slices"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/domainfront"
	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

const (
	// The ordinary nearby SNI search scans 256 IPs. A website lookup is a
	// deliberately smaller operation: no configured catalog or SNI history.
	redirectNearbyMaxIPs  = 32
	redirectHTTPFinalists = 16
)

// nearbyRedirectCandidates converts only verified DNS names discovered on
// neighboring public IPs to website candidates. The HTTP phase resolves the
// hostname normally, so a shared CDN certificate alone cannot make the
// destination appear reachable.
func nearbyRedirectCandidates(rows []*RealityScanResult) []domainfront.DecoyCandidate {
	out := make([]domainfront.DecoyCandidate, 0, redirectHTTPFinalists)
	seen := make(map[string]bool)
	for _, row := range rows {
		if row == nil || !row.CertValid || row.PrivateTarget || row.Port != 443 ||
			net.ParseIP(row.Host) != nil || net.ParseIP(row.IP) == nil ||
			netsafe.IsBlockedIP(net.ParseIP(row.IP)) {
			continue
		}
		host := strings.ToLower(strings.TrimSuffix(row.Host, "."))
		if !strings.Contains(host, ".") || strings.ContainsAny(host, "*/@ ") || seen[host] {
			continue
		}
		seen[host] = true
		out = append(out, domainfront.DecoyCandidate{
			URL:          "https://" + host + "/",
			SNILatencyMS: int64(row.LatencyMs),
		})
	}
	// Pick the fastest TLS-confirmed neighboring websites for the HTTP
	// check; REALITY feasibility is irrelevant for ordinary redirects.
	slices.SortStableFunc(out, func(a, b domainfront.DecoyCandidate) int {
		if a.SNILatencyMS < b.SNILatencyMS {
			return -1
		}
		if a.SNILatencyMS > b.SNILatencyMS {
			return 1
		}
		return strings.Compare(a.URL, b.URL)
	})
	if len(out) > redirectHTTPFinalists {
		out = out[:redirectHTTPFinalists]
	}
	return out
}

// ScanRedirectSites discovers certificate-backed websites on public IPs
// adjacent to this panel server's public IP, then measures actual HTTPS
// responses. It only recommends; enabling or saving a redirect is a
// separate administrator action.
func ScanRedirectSites(ctx context.Context) (domainfront.DecoySiteScan, error) {
	rows, err := (&ServerService{}).ScanNearbySNIsLimited(ctx, "", redirectNearbyMaxIPs)
	if err != nil {
		return domainfront.DecoySiteScan{}, err
	}
	return domainfront.ScanDecoySites(ctx, nearbyRedirectCandidates(rows)), nil
}
