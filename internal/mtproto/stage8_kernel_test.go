package mtproto

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStage8KernelMeko(t *testing.T) {
	if os.Getenv("STAGE8_NETNS") != "1" {
		t.Skip("requires private root network namespace")
	}
	self, e1 := os.Readlink("/proc/self/ns/net")
	host, e2 := os.Readlink("/proc/1/ns/net")
	if os.Geteuid() != 0 || e1 != nil || e2 != nil || self == host {
		t.Fatal("refusing firewall mutations outside private netns")
	}
	run := func(name string, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v %s", name, err, out)
		}
		return string(out)
	}
	run("nft", "add", "table", "inet", "stage8_administrator")
	before := run("nft", "-s", "list", "table", "inet", "stage8_administrator")
	dir := t.TempDir()
	data, err := os.ReadFile("../Telemt/telemt-meko-fix.sh")
	if err != nil {
		t.Fatal(err)
	}
	helper := filepath.Join(dir, "meko.sh")
	source := strings.ReplaceAll(string(data), "/run/lock/x-ui-telemt-meko.lock", filepath.Join(dir, "lock"))
	source = strings.ReplaceAll(source, "/etc/x-ui/telemt-meko-fix.env", filepath.Join(dir, "settings"))
	if err := os.WriteFile(helper, []byte(source), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TELEMT_PORTS", "24443,24444")
	t.Setenv("TELEMT_MEKO_ENABLED", "1")
	t.Setenv("TELEMT_MEKO_RATE", "54/minute")
	t.Setenv("TELEMT_MEKO_BURST", "1")
	t.Cleanup(func() { _ = exec.Command("bash", helper, "remove").Run() })
	run("bash", helper, "apply")
	status := run("bash", helper, "status")
	if !strings.Contains(status, "installed=true") || !strings.Contains(status, "backend=nftables") || !strings.Contains(status, "verified_ports=24443,24444") {
		t.Fatal(status)
	}
	run("bash", helper, "apply")
	run("bash", helper, "remove")
	if after := run("nft", "-s", "list", "table", "inet", "stage8_administrator"); after != before {
		t.Fatal("administrator rules changed")
	}
	if tables := run("nft", "list", "tables"); strings.Contains(tables, "xui_telemt_meko") {
		t.Fatal("MEKO tables survived removal", tables)
	}
}
