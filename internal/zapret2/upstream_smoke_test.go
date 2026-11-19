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
}
