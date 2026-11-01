package service

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

// nearbySNIAddresses follows RealiTLScanner's outward, alternating search.
// This bounded version excludes the origin and private addresses. Neighboring
// addresses do not imply geographic proximity.
func nearbySNIAddresses(origin netip.Addr, limit int) []string {
	origin = origin.Unmap()
	if !origin.IsGlobalUnicast() || netsafe.IsBlockedIP(net.ParseIP(origin.String())) || limit <= 0 {
		return nil
	}
	low, high := origin, origin
	out := make([]string, 0, limit)
	for len(out) < limit {
		low, high = low.Prev(), high.Next()
		added := false
		for _, ip := range []netip.Addr{low, high} {
			if ip.IsGlobalUnicast() && !netsafe.IsBlockedIP(net.ParseIP(ip.String())) {
				out = append(out, ip.String())
				added = true
				if len(out) == limit {
					break
				}
			}
		}
		if !added {
			break
		}
	}
	return out
}

// ScanNearbySNIs discovers certificate names without SNI, then verifies each
// name against the same IP with SNI. Keep the IP target to avoid DNS moving the
// selected REALITY destination away from the verified neighboring endpoint.
func (s *ServerService) ScanNearbySNIs(ctx context.Context, reference string) ([]*RealityScanResult, error) {
	reference = strings.TrimSpace(reference)
	if reference == "" {
		if done := s.resolvePublicIPsInBackground(); done != nil {
			select {
			case <-done:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		ipv4, ipv6 := s.publicIPs()
		reference = ipv4
		if _, err := netip.ParseAddr(reference); err != nil {
			reference = ipv6
		}
	}
	host, port, err := splitRealityTarget(reference)
	if err != nil {
		return nil, fmt.Errorf("server IP/domain required: %w", err)
	}
	lookupCtx, cancel := context.WithTimeout(ctx, realityScanTimeout)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(lookupCtx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve server address: %w", err)
	}
	var origin netip.Addr
	for _, ip := range ips {
		ip = ip.Unmap()
		if !ip.IsGlobalUnicast() || netsafe.IsBlockedIP(net.ParseIP(ip.String())) {
			return nil, fmt.Errorf("server address must be public")
		}
		if !origin.IsValid() || (!origin.Is4() && ip.Is4()) {
			origin = ip
		}
	}
	addresses := nearbySNIAddresses(origin, realityDiscoverMaxIPs)
	if len(addresses) == 0 {
		return nil, fmt.Errorf("no public server address available")
	}
	return scanNearbySNIAddresses(ctx, addresses, port, s.probeRealityAddr)
}

type nearbySNIProbe func(string, int, string, time.Duration, int, bool) *RealityScanResult

func scanNearbySNIAddresses(ctx context.Context, addresses []string, port int, probe nearbySNIProbe) ([]*RealityScanResult, error) {
	results := make([]*RealityScanResult, len(addresses))
	sem := make(chan struct{}, realityScanConcurrency)
	var wg sync.WaitGroup
	for i, ip := range addresses {
		if ctx.Err() != nil {
			break
		}
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return nil, ctx.Err()
		}
		wg.Add(1)
		go func(index int, address string) {
			defer wg.Done()
			defer func() { <-sem }()
			discovered := probe(address, port, "", realityDiscoverTimeout, 0, false)
			if discovered == nil || !discovered.CertChainValid || discovered.Host == address || ctx.Err() != nil {
				return
			}
			name := discovered.Host
			if name == "" || net.ParseIP(name) != nil || strings.Contains(name, "*") {
				return
			}
			verified := probe(address, port, name, realityDiscoverTimeout, 0, false)
			if verified != nil && verified.CertValid && verified.Host == name {
				verified.Target = net.JoinHostPort(address, strconv.Itoa(port))
				verified.IP = address
				// Only this hostname was tested with SNI, including wildcard-covered names.
				verified.ServerNames = []string{name}
				results[index] = verified
			}
		}(i, ip)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	out := dedupRealityResults(results)
	sortRealityResults(out)
	return out, nil
}
