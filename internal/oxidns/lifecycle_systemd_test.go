package oxidns

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// Runs only on a disposable Linux systemd runner as root. Never use this
// integration test on a production machine or an existing OxiDNS installation.
func TestManagedServiceLifecycle(t *testing.T) {
	if os.Getenv("OXIDNS_SYSTEMD_TEST") != "1" {
		t.Skip("requires disposable Linux systemd runner")
	}
	if os.Geteuid() != 0 {
		t.Fatal("systemd lifecycle test must run as root")
	}
	if _, err := os.Stat("/run/systemd/system"); err != nil {
		t.Fatal("runner must boot with systemd:", err)
	}
	official := os.Getenv("OXIDNS_CHECK_BINARY")
	if !filepath.IsAbs(official) {
		t.Fatal("an absolute path to the verified official binary is required")
	}
	for _, path := range []string{StateDir, servicePath} {
		if _, err := os.Lstat(path); err == nil {
			t.Fatalf("refusing to touch existing OxiDNS installation: %s", path)
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	t.Cleanup(func() {
		cleanupCtx, stop := context.WithTimeout(context.Background(), 20*time.Second)
		defer stop()
		if err := Uninstall(cleanupCtx); err != nil {
			t.Logf("cleanup after test: %v", err)
		}
	})
	executable, err := os.ReadFile(official)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(binaryPath, executable, 0755); err != nil {
		t.Fatal(err)
	}
	if err := InstallOnly(ctx); err != nil {
		t.Fatalf("register service: %v", err)
	}
	status, err := CurrentStatus(ctx)
	if err != nil || !status.Installed || status.Enabled {
		t.Fatalf("a fresh package must be installed but not active: %+v %v", status, err)
	}
	// Deterministic local name: no third-party upstream or public DNS is needed.
	configuration := `log:
  level: warn
plugins:
  - tag: local_hosts
    type: hosts
    args:
      entries:
        - "full:example.com 203.0.113.7"
      short_circuit: true
  - tag: main
    type: sequence
    args:
      - exec: "$local_hosts"
      - exec: accept
  - tag: udp_local
    type: udp_server
    args:
      entry: main
      listen: "127.0.0.1:15353"
  - tag: tcp_local
    type: tcp_server
    args:
      entry: main
      listen: "127.0.0.1:15353"
`
	if err := atomicWrite(configPath, []byte(configuration), 0600); err != nil {
		t.Fatal(err)
	}
	settings := Defaults()
	settings.Enabled = true
	settings.DoH = true
	settings.Domain = "dns.example.com"
	data, err := json.Marshal(settings)
	if err != nil { t.Fatal(err) }
	if err := atomicWrite(statePath, data, 0600); err != nil { t.Fatal(err) }
	check := exec.CommandContext(ctx, binaryPath, "check", "-c", configPath)
	if output, err := check.CombinedOutput(); err != nil {
		t.Fatalf("official parser rejected lifecycle config: %v (%s)", err, output)
	}
	if err := systemctl(ctx, "enable", "--now", "x-ui-oxidns.service"); err != nil {
		t.Fatal(err)
	}
	if err := waitHealthy(ctx); err != nil {
		t.Fatalf("real DNS query through managed systemd service failed: %v", err)
	}
	status, err = CurrentStatus(ctx)
	if err != nil || !status.Installed || !status.Enabled || !status.Running {
		t.Fatalf("installed service should be running: %+v %v", status, err)
	}
	if err := Uninstall(ctx); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	for _, path := range []string{binaryPath, configPath, servicePath, statePath, filepath.Join(StateDir,"data")} {
		if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("managed file remains after removal: %s (%v)", path, err)
		}
	}
	status, err = CurrentStatus(ctx)
	if err != nil || status.Installed || status.Enabled || status.Running || status.DoH || status.DoHURL != "" {
		t.Errorf("DNS feature must disappear from status after removal: %+v %v", status, err)
	}
	if err := exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", "x-ui-oxidns.service").Run(); err == nil {
		t.Error("OxiDNS service still active after removal")
	}
	if err := Uninstall(ctx); err != nil {
		t.Errorf("uninstall should be idempotent: %v", err)
	}
}
