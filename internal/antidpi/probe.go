// Package antidpi provides bounded, read-only connectivity diagnostics.
package antidpi

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var hostname = regexp.MustCompile(`^[a-zA-Z0-9](?:[a-zA-Z0-9.-]{0,251}[a-zA-Z0-9])?$`)

type Observation struct {
	IP         string `json:"ip"`
	TCP        string `json:"tcp"`
	TLS        string `json:"tls"`
	Detail     string `json:"detail,omitempty"`
	DurationMS int64  `json:"durationMs"`
}

type Report struct {
	Scope        string        `json:"scope"`
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	CheckedAt    time.Time     `json:"checkedAt"`
	Observations []Observation `json:"observations"`
}

var excluded = []netip.Prefix{
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("2002::/16"),
	netip.MustParsePrefix("2001::/32"),
}

func publicIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	if !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return false
	}
	for _, prefix := range excluded {
		if prefix.Contains(ip) {
			return false
		}
	}
	return true
}

func validHost(host string) bool {
	if !hostname.MatchString(host) || !strings.Contains(host, ".") || net.ParseIP(host) != nil {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
	}
	return true
}

// Probe resolves once and dials only the checked public addresses. It does not
// follow redirects, invoke commands, modify firewall rules or send credentials.
func Probe(ctx context.Context, host string, port int) (Report, error) {
	host = strings.TrimSpace(host)
	if !validHost(host) || port != 443 {
		return Report{}, fmt.Errorf("provide a public DNS hostname and HTTPS port 443")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host+".")
	if err != nil {
		return Report{}, fmt.Errorf("DNS lookup: %w", err)
	}
	if len(ips) == 0 || len(ips) > 8 {
		return Report{}, fmt.Errorf("expected 1..8 DNS addresses")
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return Report{}, fmt.Errorf("DNS contains a non-public address")
		}
	}
	report := Report{Scope: "server", Host: host, Port: port, CheckedAt: time.Now().UTC(), Observations: []Observation{}}
	err = collectObservations(ctx, &report, ips, probeAddress)
	return report, err
}

// collectObservations preserves completed observations, but never reports
// success when the overall request was canceled during the last address.
func collectObservations(ctx context.Context, report *Report, ips []netip.Addr, probe func(context.Context, string, netip.Addr, int) Observation) error {
	for _, ip := range ips {
		if err := ctx.Err(); err != nil {
			return err
		}
		report.Observations = append(report.Observations, probe(ctx, report.Host, ip.Unmap(), report.Port))
	}
	return ctx.Err()
}

func probeAddress(ctx context.Context, host string, ip netip.Addr, port int) Observation {
	start := time.Now()
	result := Observation{IP: ip.String(), TCP: "failed", TLS: "not_tested"}
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), strconv.Itoa(port)))
	if err == nil {
		defer conn.Close()
		result.TCP = "ok"
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		err = tlsConn.HandshakeContext(ctx)
		result.TLS = "failed"
		if err == nil {
			result.TLS = "ok"
		}
	}
	if err != nil {
		result.Detail = err.Error()
	}
	result.DurationMS = time.Since(start).Milliseconds()
	return result
}
