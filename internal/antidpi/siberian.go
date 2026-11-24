package antidpi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strings"
	"sync"
	"time"

	tls "github.com/refraction-networking/utls"
)

// SiberianOptions deliberately requires two operator-owned HTTPS names on the
// same public IP. This avoids random SNI and disabled certificate verification.
// The test is an adaptation of Siberian Check, not an exact reproduction.
type SiberianOptions struct {
	Host        string `json:"host"`
	ControlHost string `json:"controlHost"`
	Fingerprint string `json:"fingerprint"`
}

type TLSAttempt struct {
	Stage      string `json:"stage"`
	Outcome    string `json:"outcome"`
	Detail     string `json:"detail,omitempty"`
	DurationMS int64  `json:"durationMs"`
}

type SiberianReport struct {
	Scope        string       `json:"scope"`
	Host         string       `json:"host"`
	ControlHost  string       `json:"controlHost"`
	IP           string       `json:"ip"`
	Fingerprint  string       `json:"fingerprint"`
	Baseline     []TLSAttempt `json:"baseline"`
	Burst        []TLSAttempt `json:"burst"`
	Control      *TLSAttempt  `json:"control,omitempty"`
	Evidence     string       `json:"evidence"`
}

func fingerprint(name string) (tls.ClientHelloID, bool) {
	switch name {
	case "chrome":
		return tls.HelloChrome_Auto, true
	case "firefox":
		return tls.HelloFirefox_Auto, true
	case "safari":
		return tls.HelloSafari_Auto, true
	case "android":
		return tls.HelloAndroid_11_OkHttp, true
	default:
		return tls.ClientHelloID{}, false
	}
}

func resolvePublic(ctx context.Context, host string) ([]netip.Addr, error) {
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host+".")
	if err != nil {
		return nil, err
	}
	if len(ips) == 0 || len(ips) > 8 {
		return nil, errors.New("expected 1..8 DNS addresses")
	}
	for i, ip := range ips {
		ips[i] = ip.Unmap()
		if !publicIP(ips[i]) {
			return nil, errors.New("DNS contains non-public address")
		}
	}
	return ips, nil
}

func sharedAddress(a, b []netip.Addr) (netip.Addr, error) {
	// Require a unique common IP; do not silently choose a CDN address.
	seen := make(map[netip.Addr]bool)
	for _, ip := range a {
		for _, other := range b {
			if ip == other {
				seen[ip] = true
			}
		}
	}
	if len(seen) != 1 {
		return netip.Addr{}, errors.New("the two names must resolve to exactly one common public IP")
	}
	for ip := range seen {
		return ip, nil
	}
	return netip.Addr{}, errors.New("no common address")
}

func runTLSAttempt(ctx context.Context, ip netip.Addr, host string, fp tls.ClientHelloID) TLSAttempt {
	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	a := TLSAttempt{Stage: "tcp", Outcome: "ok"}
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(ip.String(), "443"))
	if err == nil {
		defer conn.Close()
		a.Stage = "tls"
		client := tls.UClient(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}, fp)
		err = client.HandshakeContext(ctx)
	}
	if err != nil {
		a.Outcome = "error"
		var ne net.Error
		if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &ne) && ne.Timeout()) {
			a.Outcome = "timeout"
		}
		a.Detail = err.Error()
	}
	a.DurationMS = time.Since(start).Milliseconds()
	return a
}

func classifySiberian(r SiberianReport) string {
	if len(r.Baseline) != 2 || len(r.Burst) != 4 || r.Control == nil {
		return "inconclusive"
	}
	for _, a := range r.Baseline {
		if a.Outcome != "ok" {
			return "inconclusive"
		}
	}
	if r.Control.Outcome != "ok" {
		return "inconclusive"
	}
	timeouts := 0
	for _, a := range r.Burst {
		if a.Stage == "tls" && a.Outcome == "timeout" {
			timeouts++
		} else if a.Outcome != "ok" {
			return "inconclusive"
		}
	}
	if timeouts > 0 {
		return "asymmetry"
	}
	return "no-signal"
}

// CheckSiberian runs one bounded comparison. No packets other than ordinary
// TCP and verified TLS handshakes are generated. It measures the panel's
// network, not the subscriber's network. A timeout pattern is not proof.
func CheckSiberian(ctx context.Context, opt SiberianOptions) (SiberianReport, error) {
	host := strings.ToLower(strings.TrimSpace(opt.Host))
	control := strings.ToLower(strings.TrimSpace(opt.ControlHost))
	fp, ok := fingerprint(opt.Fingerprint)
	if !ok || !validHost(host) || !validHost(control) || host == control {
		return SiberianReport{}, errors.New("provide two distinct DNS hostnames and a supported fingerprint")
	}
	ctx, cancel := context.WithTimeout(ctx, 28*time.Second)
	defer cancel()
	a, err := resolvePublic(ctx, host)
	if err != nil {
		return SiberianReport{}, fmt.Errorf("target DNS: %w", err)
	}
	b, err := resolvePublic(ctx, control)
	if err != nil {
		return SiberianReport{}, fmt.Errorf("control DNS: %w", err)
	}
	ip, err := sharedAddress(a, b)
	if err != nil {
		return SiberianReport{}, err
	}
	r := SiberianReport{Scope: "server", Host: host, ControlHost: control, IP: ip.String(), Fingerprint: opt.Fingerprint, Evidence: "inconclusive"}
	r.Baseline = append(r.Baseline, runTLSAttempt(ctx, ip, host, fp))
	r.Baseline = append(r.Baseline, runTLSAttempt(ctx, ip, control, fp))
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	if r.Baseline[0].Outcome != "ok" || r.Baseline[1].Outcome != "ok" {
		return r, nil
	}
	r.Burst = make([]TLSAttempt, 4)
	var wg sync.WaitGroup
	for i := range r.Burst {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r.Burst[i] = runTLSAttempt(ctx, ip, host, fp)
		}(i)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	result := runTLSAttempt(ctx, ip, control, fp)
	r.Control = &result
	if ctx.Err() != nil {
		return r, ctx.Err()
	}
	r.Evidence = classifySiberian(r)
	return r, nil
}
