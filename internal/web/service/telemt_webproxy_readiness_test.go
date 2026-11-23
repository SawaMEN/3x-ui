package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTelemtWebHealthyDecoyStatus(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   bool
	}{
		{http.StatusOK, true},
		{http.StatusNoContent, true},
		{http.StatusMovedPermanently, true},
		{http.StatusFound, true},
		{http.StatusUnauthorized, true},
		{http.StatusForbidden, true},
		{http.StatusNotFound, true},
		{http.StatusMethodNotAllowed, true},
		{http.StatusBadRequest, false},
		{http.StatusTooManyRequests, false},
		{http.StatusInternalServerError, false},
		{http.StatusBadGateway, false},
		{http.StatusServiceUnavailable, false},
	} {
		if got := telemtWebHealthyDecoyStatus(tc.status); got != tc.want {
			t.Errorf("HTTP %d: healthy=%t want %t", tc.status, got, tc.want)
		}
	}
}

func TestPreparedTelemtWebSiteRequiresSuccessfulRoot(t *testing.T) {
	for _, status := range []int{301, 403, 404, 405, 502} {
		if telemtWebSiteReady(TelemtWebProxyState{}, status) {
			t.Errorf("prepared site incorrectly marked ready on HTTP %d", status)
		}
	}
	if !telemtWebSiteReady(TelemtWebProxyState{}, 200) ||
		!telemtWebSiteReady(TelemtWebProxyState{Site: &TelemtWebSiteOptions{Mode: "origin"}}, 302) {
		t.Fatal("ready site incorrectly marked unavailable")
	}
}

// A valid primary host must not hide a broken alias.
type telemtWebTestTransport func(*http.Request) (*http.Response, error)

func (f telemtWebTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestTelemtWebProbeChecksEveryHost(t *testing.T) {
	state := TelemtWebProxyState{Domain: "example.com", Aliases: []string{"alias.example.com"}}
	var hosts []string
	aliasStatus := http.StatusBadGateway
	client := &http.Client{
		Transport: telemtWebTestTransport(func(r *http.Request) (*http.Response, error) {
			hosts = append(hosts, r.URL.Host)
			if r.Host != r.URL.Host || r.URL.Path != "/" || r.URL.Scheme != "https" {
				t.Fatalf("unexpected request: %s %s", r.Host, r.URL)
			}
			status := http.StatusOK
			if r.URL.Host == state.Aliases[0] {
				status = aliasStatus
			}
			return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader("site")), Header: make(http.Header)}, nil
		}),
	}
	err := telemtWebProbeHosts(context.Background(), state, client)
	if err == nil || !strings.Contains(err.Error(), "alias.example.com") || !strings.Contains(err.Error(), "502") {
		t.Fatalf("alias failure was missed: %v", err)
	}
	if strings.Join(hosts, ",") != "example.com,alias.example.com" {
		t.Fatalf("unexpected hosts: %v", hosts)
	}
	aliasStatus = http.StatusOK
	if err := telemtWebProbeHosts(context.Background(), state, client); err != nil {
		t.Fatal(err)
	}
}

func TestTelemtWebIPv6AdvertisedAddressNeedsIPv6Listener(t *testing.T) {
	if !telemtWebNeedsIPv6(TelemtWebProxyState{PublicAddr: "[2001:db8::1]:443"}) {
		t.Fatal("IPv6 endpoint requires IPv6 HTTPS listener")
	}
	if telemtWebNeedsIPv6(TelemtWebProxyState{}) {
		t.Fatal("empty endpoint must not change default listener")
	}
}
