package zapret2

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// This test uses the genuine latest official GitHub release. It is opt-in:
// ordinary unit tests use a hermetic fixture and do not depend on the network.
func TestOfficialNfqws2DryRunProfiles(t *testing.T) {
	if os.Getenv("ZAPRET2_UPSTREAM_SMOKE") != "1" {
		t.Skip("requires an official zapret2 binary installed by zapret2-smoke workflow")
	}
	if !Installed() {
		t.Fatal("official nfqws2 or required Lua scripts are not installed")
	}
	for _, test := range []struct {
		name    string
		profile string
		tcp     bool
		udp     bool
	}{
		{"safe-tcp", "safe", true, false},
		{"balanced-tcp", "balanced", true, false},
		{"aggressive-tcp", "aggressive", true, false},
		{"auto-tcp-udp", "auto", true, true},
		{"udp-only", "safe", false, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			p, err := Parse(test.name, map[string]any{
				"port": 15900,
				"profile": test.profile,
				"tcp": test.tcp,
				"udp": test.udp,
			})
			if err != nil {
				t.Fatal(err)
			}
			p.Queue = 2400
			args, err := strategyArgs(p)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, BinaryPath(), append([]string{"--dry-run"}, args...)...)
			cmd.Dir = Directory()
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("official nfqws2 rejected %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
			}
		})
	}
	t.Run("calibrated-http-tls-quic-ipv4-ipv6", func(t *testing.T) {
		calibration := &Calibration{
			Source: "blockcheck2",
			TestedAt: time.Now().UTC().Format(time.RFC3339),
			Domains: []string{"example.com"},
			Level: "standard",
			Selected: []SelectedStrategy{
				{Kind: "http", IP: "4", Test: "curl_test_http", Args: []string{"--payload=http_req", "--lua-desync=http_hostcase"}, Passed: 1, Total: 1},
				{Kind: "https", IP: "4", Test: "curl_test_https_tls12", Args: []string{"--payload=tls_client_hello", "--lua-desync=multisplit:pos=1,midsld"}, Passed: 1, Total: 1},
				{Kind: "quic", IP: "6", Test: "curl_test_http3", Args: []string{"--payload=quic_initial", "--lua-desync=fake:blob=fake_default_quic:repeats=5"}, Passed: 1, Total: 1},
			},
		}
		p := Plan{Tag: "test-auto", Queue: 2400, Options: Options{Port: 15900, Profile: "auto", TCP: true, UDP: true, Calibration: calibration}}
		args, err := strategyArgs(p)
		if err != nil {t.Fatal(err)}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, BinaryPath(), append([]string{"--dry-run"}, args...)...)
		cmd.Dir = Directory()
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("official nfqws2 rejected calibrated args %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
		}
	})
}
