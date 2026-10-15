package controller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

type outboundDelayStub struct {
	calls    []string
	timeouts []time.Duration
	failures map[string]error
}

func (s *outboundDelayStub) ProxyDelay(_ context.Context, _ string, target string, timeout time.Duration) (*singbox.ClashProxyDelay, error) {
	s.calls = append(s.calls, target)
	s.timeouts = append(s.timeouts, timeout)
	if err := s.failures[target]; err != nil {
		return nil, err
	}
	return &singbox.ClashProxyDelay{Delay: 71}, nil
}

func TestWARPProbeFallsBackToIndependentHTTPSHost(t *testing.T) {
	probe := &outboundDelayStub{failures: map[string]error{
		warpOutboundProbeURL: errors.New("cloudflare HTTPS probe unavailable"),
	}}
	result, target, err := measureOutboundDelay(
		context.Background(), probe, "warp-masque", defaultOutboundProbeURL,
		10*time.Second, true, false,
	)
	if err != nil || result == nil || result.Delay != 71 || target != defaultOutboundProbeURL {
		t.Fatalf("fallback probe = %+v, %q, %v", result, target, err)
	}
	if len(probe.calls) != 2 || probe.calls[0] != warpOutboundProbeURL || probe.calls[1] != defaultOutboundProbeURL {
		t.Fatalf("wrong fallback order: %v", probe.calls)
	}
	if probe.timeouts[0] != 5*time.Second || probe.timeouts[1] != 5*time.Second {
		t.Fatalf("probe exceeded its timeout budget: %v", probe.timeouts)
	}
}

func TestWARPProbeReportsBothFailures(t *testing.T) {
	probe := &outboundDelayStub{failures: map[string]error{
		warpOutboundProbeURL: errors.New("warp handshake failed"),
		defaultOutboundProbeURL: errors.New("no egress"),
	}}
	_, _, err := measureOutboundDelay(
		context.Background(), probe, "warp-masque", defaultOutboundProbeURL,
		10*time.Second, true, false,
	)
	if err == nil {
		t.Fatal("two failed HTTPS probes must be reported")
	}
	for _, want := range []string{warpOutboundProbeURL, defaultOutboundProbeURL, "warp handshake failed", "no egress", "HTTP/2"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("missing %q from error: %v", want, err)
		}
	}
}

func TestOutboundProbeRespectsCustomURLAndOtherTypes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		warp      bool
		customURL bool
	}{
		{"normal outbound", false, false},
		{"custom WARP target", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe := &outboundDelayStub{failures: map[string]error{
				"https://example.com/test": errors.New("unavailable"),
			}}
			_, _, err := measureOutboundDelay(
				context.Background(), probe, "probe", "https://example.com/test",
				8*time.Second, tc.warp, tc.customURL,
			)
			if err == nil || len(probe.calls) != 1 || probe.timeouts[0] != 8*time.Second {
				t.Fatalf("unexpected alternate target or timeout: %v, %v, %v", probe.calls, probe.timeouts, err)
			}
		})
	}
}
