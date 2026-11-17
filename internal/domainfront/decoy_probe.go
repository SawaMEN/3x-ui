package domainfront

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

// Candidates are discovered by the existing nearby-IP SNI search.
// Only the URL and measured SNI latency are needed for website selection.
type DecoyCandidate struct {
	URL          string
	SNILatencyMS int64
}

type DecoySiteCheck struct {
	URL          string `json:"url"`
	SNILatencyMS int64  `json:"sniLatencyMs"`
	Available    bool   `json:"available"`
	Status       int    `json:"status"`
	PathStatus   int    `json:"pathStatus"`
	LatencyMS    int64  `json:"latencyMs"`
	Error        string `json:"error,omitempty"`
}

type DecoySiteScan struct {
	Recommended string           `json:"recommended"`
	CheckedAt   int64            `json:"checkedAt"`
	Sites       []DecoySiteCheck `json:"sites"`
}

// Includes address ranges that IsPrivate/IsGlobalUnicast do not reject on all
// Go versions (CGNAT, benchmarking, documentation and special-use networks).
var forbiddenDecoyNets = func() []netip.Prefix {
	values := []string{
		"0.0.0.0/8", "100.64.0.0/10", "169.254.0.0/16",
		"192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4",
		"240.0.0.0/4", "2001:db8::/32", "2001::/32", "fc00::/7", "fe80::/10",
	}
	out := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		out = append(out, netip.MustParsePrefix(value))
	}
	return out
}()

func publicDecoyIP(ip net.IP) bool {
	addr, ok := netip.AddrFromSlice(ip)
	if !ok {
		return false
	}
	addr = addr.Unmap()
	if netsafe.IsBlockedIP(ip) {
		return false
	}
	for _, prefix := range forbiddenDecoyNets {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

// dialPublicDecoy prevents proxy use and DNS rebinding to local or reserved
// networks. The transport still authenticates the requested HTTPS hostname.
func dialPublicDecoy(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port != "443" {
		return nil, errors.New("decoy probe requires HTTPS/443")
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	dialer := net.Dialer{Timeout: 2500 * time.Millisecond}
	for _, entry := range ips {
		if !publicDecoyIP(entry.IP) {
			continue
		}
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(entry.IP.String(), port))
		if err == nil {
			return conn, nil
		}
	}
	return nil, fmt.Errorf("no reachable public address for %s", host)
}

func decoyHTTPClient() *http.Client {
	transport := &http.Transport{
		// Never use a system proxy: probe the server's direct path.
		Proxy:                 nil,
		DisableKeepAlives:     true,
		TLSHandshakeTimeout:   2500 * time.Millisecond,
		ResponseHeaderTimeout: 2500 * time.Millisecond,
		DialContext:           dialPublicDecoy,
	}
	return &http.Client{
		Timeout:   4 * time.Second,
		Transport: transport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			// A redirect can point at internal networks. Observe only the
			// first response and never follow a remote Location header.
			return http.ErrUseLastResponse
		},
	}
}

func decoyHTTPStatus(ctx context.Context, client *http.Client, raw string) (int, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("Accept", "text/html,*/*;q=0.8")
	req.Header.Set("User-Agent", "Mozilla/5.0")
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return 0, elapsed, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, elapsed, nil
}

// A suitable website must respond on HTTPS. Sort by observed HTTP latency,
// using the neighboring-IP SNI handshake time only to break ties.
func rankDecoySites(sites []DecoySiteCheck) {
	sort.SliceStable(sites, func(i, j int) bool {
		a, b := sites[i], sites[j]
		if a.Available != b.Available {
			return a.Available
		}
		if a.Available {
			if a.LatencyMS != b.LatencyMS {
				return a.LatencyMS < b.LatencyMS
			}
			if a.SNILatencyMS != b.SNILatencyMS {
				return a.SNILatencyMS < b.SNILatencyMS
			}
		}
		return strings.Compare(a.URL, b.URL) < 0
	})
}

func probeDecoySite(ctx context.Context, client *http.Client, candidate DecoyCandidate) DecoySiteCheck {
	result := DecoySiteCheck{
		URL:          candidate.URL,
		SNILatencyMS: candidate.SNILatencyMS,
	}
	status, latency, err := decoyHTTPStatus(ctx, client, candidate.URL)
	result.Status, result.LatencyMS = status, latency.Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if status >= 200 && status < 400 {
		base, err := url.Parse(candidate.URL)
		if err == nil {
			base.Path = "/123"
			base.RawQuery = ""
			result.PathStatus, _, _ = decoyHTTPStatus(ctx, client, base.String())
		}
	}
	result.Available = result.Status >= 200 && result.Status < 400
	return result
}

// ScanDecoySites measures real HTTPS latency for neighboring-IP SNI results.
// Reject custom paths, ports, credentials and non-HTTPS schemes. The HTTP API
// does not accept destination URLs, preventing arbitrary server-side fetches.
func ScanDecoySites(ctx context.Context, candidates []DecoyCandidate) DecoySiteScan {
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	out := DecoySiteScan{CheckedAt: time.Now().Unix(), Sites: []DecoySiteCheck{}}
	validated := make([]DecoyCandidate, 0, len(candidates))
	seen := make(map[string]bool)
	for _, candidate := range candidates {
		u, err := ValidateRootRedirectURL(candidate.URL)
		if err != nil || u.Scheme != "https" || u.Port() != "" ||
			u.RawQuery != "" || u.Fragment != "" ||
			(u.Path != "" && u.Path != "/") || net.ParseIP(u.Hostname()) != nil ||
			!strings.Contains(u.Hostname(), ".") {
			continue
		}
		candidate.URL = "https://" + u.Hostname() + "/"
		if seen[candidate.URL] {
			continue
		}
		seen[candidate.URL] = true
		validated = append(validated, candidate)
		if len(validated) == 16 {
			break
		}
	}
	out.Sites = make([]DecoySiteCheck, len(validated))
	client := decoyHTTPClient()
	defer client.CloseIdleConnections()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, candidate := range validated {
		wg.Add(1)
		go func(i int, candidate DecoyCandidate) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				out.Sites[i] = DecoySiteCheck{URL: candidate.URL, Error: "scan timeout"}
				return
			}
			out.Sites[i] = probeDecoySite(ctx, client, candidate)
		}(i, candidate)
	}
	wg.Wait()
	rankDecoySites(out.Sites)
	if len(out.Sites) > 0 && out.Sites[0].Available {
		out.Recommended = out.Sites[0].URL
	}
	return out
}
