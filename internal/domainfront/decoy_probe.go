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
)

// Decoy sites are deliberately server-maintained. Never dial user-supplied URLs:
// a public admin API that fetches arbitrary URLs is an SSRF risk.
var decoyCandidates = []string{
	"https://ya.ru/",
	"https://www.wikipedia.org/",
	"https://www.mozilla.org/",
	"https://github.com/",
	"https://habr.com/",
	"https://www.cloudflare.com/",
	"https://www.google.com/",
	"https://www.bing.com/",
	"https://www.microsoft.com/",
	"https://www.debian.org/",
	"https://hh.ru/",
	"https://www.ozon.ru/",
	"https://www.avito.ru/",
	"https://www.rbc.ru/",
	"https://www.2gis.ru/",
	"https://www.vk.com/",
}

type DecoySiteCheck struct {
	URL        string `json:"url"`
	Available  bool   `json:"available"`
	Status     int    `json:"status"`
	PathStatus int    `json:"pathStatus"`
	LatencyMS  int64  `json:"latencyMs"`
	Score      int    `json:"score"`
	Error      string `json:"error,omitempty"`
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
	if !addr.IsGlobalUnicast() || addr.IsPrivate() || addr.IsLoopback() {
		return false
	}
	for _, prefix := range forbiddenDecoyNets {
		if prefix.Contains(addr) {
			return false
		}
	}
	return true
}

func decoyHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: nil, // Probe the server's direct path, not a system proxy.
		DisableKeepAlives: true,
		MaxIdleConns: 0,
		TLSHandshakeTimeout: 2500 * time.Millisecond,
		ResponseHeaderTimeout: 2500 * time.Millisecond,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
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
		},
	}
	return &http.Client{
		Timeout: 4 * time.Second,
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
	req.Header.Set("User-Agent", "3x-ui-decoy-check/1.0")
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return 0, elapsed, err
	}
	_ = resp.Body.Close()
	return resp.StatusCode, elapsed, nil
}

func scoreDecoySite(site *DecoySiteCheck) {
	site.Available = site.Status >= 200 && site.Status < 400
	if !site.Available {
		site.Score = 0
		return
	}
	// HTTPS response success, server-side latency and unknown-path behavior.
	score := 70
	switch {
	case site.PathStatus >= 200 && site.PathStatus < 400:
		score += 20
	case site.PathStatus == http.StatusNotFound:
		score += 5 // A real website can legitimately return 404.
	case site.PathStatus > 0:
		score -= 5
	}
	score -= int(min(site.LatencyMS/50, 20))
	if score < 1 {
		score = 1
	}
	site.Score = score
}

func probeDecoySite(ctx context.Context, client *http.Client, raw string) DecoySiteCheck {
	result := DecoySiteCheck{URL: raw}
	status, latency, err := decoyHTTPStatus(ctx, client, raw)
	result.Status, result.LatencyMS = status, latency.Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	if status >= 200 && status < 400 {
		base, err := url.Parse(raw)
		if err == nil {
			base.Path = "/robots.txt"
			base.RawQuery = ""
			result.PathStatus, _, _ = decoyHTTPStatus(ctx, client, base.String())
		}
	}
	scoreDecoySite(&result)
	return result
}

// ScanDecoySites checks only a finite allowlisted set. Scores reflect
// reachability from THIS server, not accessibility from clients' networks,
// blocklist status or any guarantee of censorship circumvention.
func ScanDecoySites(ctx context.Context) DecoySiteScan {
	ctx, cancel := context.WithTimeout(ctx, 18*time.Second)
	defer cancel()
	out := DecoySiteScan{CheckedAt: time.Now().Unix(), Sites: make([]DecoySiteCheck, len(decoyCandidates))}
	client := decoyHTTPClient()
	defer client.CloseIdleConnections()
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, candidate := range decoyCandidates {
		wg.Add(1)
		go func(i int, raw string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				out.Sites[i] = DecoySiteCheck{URL: raw, Error: "scan timeout"}
				return
			}
			out.Sites[i] = probeDecoySite(ctx, client, raw)
		}(i, candidate)
	}
	wg.Wait()
	sort.SliceStable(out.Sites, func(i, j int) bool {
		a, b := out.Sites[i], out.Sites[j]
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.LatencyMS != b.LatencyMS {
			return a.LatencyMS < b.LatencyMS
		}
		return strings.Compare(a.URL, b.URL) < 0
	})
	if len(out.Sites) > 0 && out.Sites[0].Available {
		out.Recommended = out.Sites[0].URL
	}
	return out
}
