package externalvpn

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"time"
)

// These fixed public hosts are the entire diagnostic allowlist. Never dial a
// user-supplied channel URL: the panel must not become an SSRF proxy.
var openFluxProbeHosts = map[string]string{
	"yandex":     "docs.yandex.ru",
	"vyandex":    "docs.yandex.ru",
	"boards":     "boards.yandex.ru",
	"mailru":     "docs.mail.ru",
	"cupsonline": "cups.online",
	"avito":      "code.avito.ru",
	"telemost":   "telemost.yandex.ru",
	"oneme":      "ws-api.oneme.ru",
}

type OpenFluxProbeResult struct {
	Host      string `json:"host"`
	Reachable bool   `json:"reachable"`
	Scope     string `json:"scope"`
}

func openFluxProbeHost(transport string) (string, bool) {
	host, ok := openFluxProbeHosts[transport]
	return host, ok
}

// ProbeOpenFluxProvider verifies only outbound DNS, TCP and certificate-checked
// TLS to a provider. It cannot authenticate a shared document or a MAX account,
// and it must never be reported as a successful end-to-end OpenFlux session.
func ProbeOpenFluxProvider(ctx context.Context, transport string) (OpenFluxProbeResult, error) {
	host, ok := openFluxProbeHost(transport)
	if !ok {
		return OpenFluxProbeResult{}, fmt.Errorf("this OpenFlux transport has no provider-side connectivity check")
	}
	result := OpenFluxProbeResult{Host: host, Scope: "provider-tls"}
	if !OpenFluxBinaryStatus(ctx, true).Installed {
		return result, fmt.Errorf("OpenFlux is not installed on this server")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	dialer := &tls.Dialer{
		NetDialer: &net.Dialer{Timeout: 4 * time.Second},
		Config:    &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12},
	}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, "443"))
	if err != nil {
		return result, fmt.Errorf("cannot establish TLS connection to %s: %w", host, err)
	}
	_ = conn.Close()
	result.Reachable = true
	return result, nil
}
