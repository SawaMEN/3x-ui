package service

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/util/netsafe"
)

type HTTP01Observation struct {
	Name      string   `json:"name"`
	CheckedAt int64    `json:"checkedAt"`
	Addresses []string `json:"addresses"`
	Verified  bool     `json:"verified"`
	Detail    string   `json:"detail"`
}

var observedToken = regexp.MustCompile(`^3xui-reach-[a-f0-9]{48}$`)

// ObservePublicHTTP01 is used only by trusted, authenticated child nodes as an
// independent network vantage. It does not contact any private/reserved
// address, follow redirects, fetch arbitrary paths or request certificates.
func ObservePublicHTTP01(ctx context.Context, name, token string) (HTTP01Observation, error) {
	result := HTTP01Observation{Name: name, CheckedAt: time.Now().Unix(), Addresses: []string{}}
	name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(name), "."))
	if len(name) > 253 || len(name) < 4 || !strings.Contains(name, ".") ||
		net.ParseIP(name) != nil || !observedToken.MatchString(token) {
		return result, errors.New("invalid HTTP-01 observation request")
	}
	for _, part := range strings.Split(name, ".") {
		if len(part) < 1 || len(part) > 63 || part[0] == '-' || part[len(part)-1] == '-' {
			return result, errors.New("invalid HTTP-01 observation hostname")
		}
		for _, ch := range part {
			if ch != '-' && (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') {
				return result, errors.New("invalid HTTP-01 observation hostname")
			}
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, name)
	if err != nil {
		result.Detail = "DNS lookup failed from the observer node"
		return result, nil
	}
	for _, ip := range ips {
		if netsafe.IsBlockedIP(ip.IP) {
			result.Detail = "Observer DNS includes a non-public address; refusing the HTTP connection"
			return result, nil
		}
		result.Addresses = append(result.Addresses, ip.IP.String())
	}
	if len(result.Addresses) == 0 || len(result.Addresses) > 16 {
		result.Detail = "Observer DNS returned no usable addresses"
		return result, nil
	}
	transport := &http.Transport{
		Proxy:             nil,
		DisableKeepAlives: true,
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return netsafe.SSRFGuardedDialContext(ctx, "tcp", addr)
		},
	}
	defer transport.CloseIdleConnections()
	client := http.Client{
		Transport: transport, Timeout: 5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("HTTP-01 diagnostic redirect refused")
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"http://"+name+"/.well-known/acme-challenge/"+token, nil)
	if err != nil {
		return result, fmt.Errorf("invalid HTTP-01 probe URL")
	}
	req.Header.Set("Cache-Control", "no-store")
	resp, err := client.Do(req)
	if err != nil {
		result.Detail = "Public TCP 80 did not return a valid HTTP-01 proof from this observer"
		return result, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("X-3xui-Reachability") != token {
		result.Detail = "Public HTTP-01 path did not return the expected proof"
		return result, nil
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 256))
	if err != nil || strings.TrimSpace(string(raw)) != token {
		result.Detail = "HTTP-01 proof content does not match"
		return result, nil
	}
	result.Verified = true
	result.Detail = "HTTP-01 challenge reachable from this independent observer"
	return result, nil
}
