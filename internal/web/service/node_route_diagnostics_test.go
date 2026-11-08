package service

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

type nodeRouteDelayStub struct {
	failFirst bool
	failAll   bool
	nilDelay  bool
	calls     []string
	tags      []string
}

func (s *nodeRouteDelayStub) ProxyDelay(_ context.Context, tag, target string, _ time.Duration) (*singbox.ClashProxyDelay, error) {
	s.tags = append(s.tags, tag)
	s.calls = append(s.calls, target)
	if s.failAll || s.failFirst && len(s.calls) == 1 {
		return nil, errors.New("general SOCKS server failure")
	}
	if s.nilDelay {
		return nil, nil
	}
	return &singbox.ClashProxyDelay{Delay: 28}, nil
}

func TestProbeNodeRouteAppliedHTTP(t *testing.T) {
	for _, tc := range []struct {
		name      string
		stub      nodeRouteDelayStub
		wantOK    bool
		wantCalls int
	}{
		{name: "first endpoint works", wantOK: true, wantCalls: 1},
		{name: "fallback endpoint works", stub: nodeRouteDelayStub{failFirst: true}, wantOK: true, wantCalls: 2},
		{name: "both endpoints fail", stub: nodeRouteDelayStub{failAll: true}, wantCalls: 2},
		{name: "no delay returned", stub: nodeRouteDelayStub{nilDelay: true}, wantCalls: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			probe, err := probeNodeRouteAppliedHTTP(context.Background(), "node-route-9", &tc.stub)
			if err != nil {
				t.Fatal(err)
			}
			if probe == nil || probe.Success != tc.wantOK || len(tc.stub.calls) != tc.wantCalls {
				t.Fatalf("unexpected probe=%+v calls=%v", probe, tc.stub.calls)
			}
			for _, tag := range tc.stub.tags {
				if tag != "node-route-9" {
					t.Fatalf("wrong outbound used: %q", tag)
				}
			}
			if tc.wantCalls == 2 && (tc.stub.calls[0] != "https://www.google.com/generate_204" ||
				tc.stub.calls[1] != "https://www.cloudflare.com/cdn-cgi/trace") {
				t.Fatalf("unexpected URL retry: %v", tc.stub.calls)
			}
		})
	}
}

func TestNodeRouteProbeFailureStage(t *testing.T) {
	cases := []struct {
		reason string
		stage  string
	}{
		{"socks connect tcp 127.0.0.1:34673->www.google.com:443: unknown error general SOCKS server failure", "proxy"},
		{"x509: certificate signed by unknown authority", "tls"},
		{"authentication failed", "auth"},
		{"lookup vpn.example: no such host", "dns"},
		{"dial tcp: connect: connection refused", "tcp"},
		{"context deadline exceeded", "timeout"},
		{"Failed to start test sing-box instance", "test-core"},
		{"other unexpected failure", "unknown"},
	}
	for _, tc := range cases {
		stage, description := nodeRouteProbeFailureStage(tc.reason)
		if stage != tc.stage || description == "" {
			t.Fatalf("reason=%q got stage=%q description=%q", tc.reason, stage, description)
		}
	}
}

func TestNodeRouteTransportEvidenceTCPDoesNotProveVPN(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	inbound := &model.Inbound{Protocol: model.VLESS, Port: port}
	native := map[string]any{"server": "127.0.0.1", "server_port": port}
	reachable, detail := nodeRouteTransportEvidence(context.Background(), inbound, native)
	if !reachable || !strings.Contains(detail, "Авторизация") {
		t.Fatalf("port check must not imply VPN authentication: %t %q", reachable, detail)
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	reachable, _ = nodeRouteTransportEvidence(context.Background(), inbound, native)
	if reachable {
		t.Fatal("closed TCP port was reported reachable")
	}
}
