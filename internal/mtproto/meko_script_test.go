package mtproto

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

// Exercise the bundled helper with a fake firewall. Concurrent invocations
// must never interleave chain rebuilds; no host firewall rules are touched.
func TestMekoHelperSerializesRuleUpdates(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux firewall helper")
	}
	for _, tool := range []string{"bash", "flock"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s is unavailable", tool)
		}
	}
	dir := t.TempDir()
	data, err := os.ReadFile("../Telemt/telemt-meko-fix.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := strings.ReplaceAll(string(data), "/run/lock/x-ui-telemt-meko.lock", filepath.Join(dir, "lock"))
	script = strings.ReplaceAll(script, "/etc/x-ui/telemt-meko-fix.env", filepath.Join(dir, "state.env"))
	helper := filepath.Join(dir, "helper.sh")
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	mock := `#!/usr/bin/env bash
set -eu
if ! mkdir "$MEKO_TEST_DIR/busy" 2>/dev/null; then
    echo overlap >> "$MEKO_TEST_DIR/errors"
    exit 1
fi
trap 'rmdir "$MEKO_TEST_DIR/busy"' EXIT
printf '%s\n' "$*" >> "$MEKO_TEST_DIR/calls"
sleep 0.005
for arg in "$@"; do
    [[ "$arg" != "-C" ]] || exit 1
done
`
	if err := os.WriteFile(filepath.Join(dir, "iptables"), []byte(mock), 0o700); err != nil {
		t.Fatal(err)
	}
	// Never invoke a real host firewall, even when nft is installed.
	if err := os.WriteFile(filepath.Join(dir, "nft"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"MEKO_TEST_DIR="+dir, "TELEMT_MEKO_ENABLED=1", "TELEMT_PORTS=443,8443,443", "TELEMT_MEKO_RATE=54/minute", "TELEMT_MEKO_BURST=1")
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			cmd := exec.Command("bash", helper, "apply")
			cmd.Env = env
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Errorf("MEKO apply failed: %v: %s", err, out)
			}
		})
	}
	wg.Wait()
	if data, err := os.ReadFile(filepath.Join(dir, "errors")); err == nil {
		t.Fatalf("firewall updates overlapped: %s", data)
	}
	calls, err := os.ReadFile(filepath.Join(dir, "calls"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"-w 2", "--set-xmark 0x400/0x400", "40 & 0xFFFF0000 = 0x02040000", "--hashlimit-upto 54/minute", "--hashlimit-burst 1", "--reject-with tcp-reset"} {
		if !strings.Contains(string(calls), want) {
			t.Errorf("MEKO V3 lost %s", want)
		}
	}
	for _, line := range strings.Split(string(calls), "\n") {
		if strings.Contains(line, "-t filter -A TELEMT_MEKO ") && !strings.Contains(line, "-j REJECT ") && !strings.HasSuffix(line, "-j RETURN") {
			t.Errorf("allowed SYN must return to the host firewall: %s", line)
		}
	}

	if strings.Count(string(calls), "--hashlimit-name tm_443 ") != 2 {
		t.Fatal("each update must deduplicate the discovered port set")
	}
}

func TestMekoNftBatchAndStatus(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux helper")
	}
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("flock unavailable")
	}
	dir := t.TempDir()
	data, err := os.ReadFile("../Telemt/telemt-meko-fix.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := strings.ReplaceAll(string(data), "/run/lock/x-ui-telemt-meko.lock", filepath.Join(dir, "lock"))
	script = strings.ReplaceAll(script, "/etc/x-ui/telemt-meko-fix.env", filepath.Join(dir, "state.env"))
	helper := filepath.Join(dir, "helper.sh")
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	mock := `#!/usr/bin/env bash
set -eu
case "$*" in
 '-f -') cat > "$MEKO_TEST_DIR/batch" ;;
 'list chain inet xui_telemt_meko input') echo 'reject with tcp reset' ;;
 '-n list set inet xui_telemt_meko ports') printf 'elements = { 443,\n 8443 }\n' ;;
 'delete table inet xui_telemt_meko') touch "$MEKO_TEST_DIR/deleted" ;;
 *) exit 1 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "nft"), []byte(mock), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iptables"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	run := func(action, enabled string) string {
		t.Helper()
		cmd := exec.Command("bash", helper, action)
		cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "MEKO_TEST_DIR="+dir, "TELEMT_PORTS=8443,443,443", "TELEMT_MEKO_ENABLED="+enabled)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s: %v %s", action, err, out)
		}
		return string(out)
	}
	run("apply", "1")
	batch, err := os.ReadFile(filepath.Join(dir, "batch"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"flush table inet xui_telemt_meko", "ports { 443,8443 }", "@th,320,32 0x04020000", "@th,160,16 0x0204", "ip saddr timeout 60s", "ip6 saddr timeout 60s", "54/minute burst 1 packets", "counter return"} {
		if !strings.Contains(string(batch), want) {
			t.Errorf("missing %s: %s", want, batch)
		}
	}
	if strings.Count(string(batch), "reject with tcp reset") != 4 {
		t.Fatal("expected one IPv4 and IPv6 limiter per port")
	}
	status := run("status", "1")
	if !strings.Contains(status, "installed=true") || !strings.Contains(status, "applied_ports=443,8443") {
		t.Fatalf("incorrect status: %s", status)
	}
	run("apply", "0")
	if _, err := os.Stat(filepath.Join(dir, "deleted")); err != nil {
		t.Fatal("disable did not remove owned nft table")
	}
}


func TestMekoOnlySelectsExplicitOrdinaryInboundPorts(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux MEKO helper")
	}
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("flock unavailable")
	}
	dir := t.TempDir()
	configDir := filepath.Join(dir, "mtproto")
	if err := os.Mkdir(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	writeConfig := func(name string, selected bool, port int) {
		t.Helper()
		body := ""
		if selected {
			body = "# x-ui-meko-fix=1\n"
		}
		body += fmt.Sprintf("[server]\nport = %d\n", port)
		if err := os.WriteFile(filepath.Join(configDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeConfig("telemt-1.toml", true, 8443)
	writeConfig("telemt-2.toml", false, 9443)
	writeConfig("telemt-3.toml", true, 7443)
	legacy := filepath.Join(dir, "telemt.toml")
	if err := os.WriteFile(legacy, []byte("[server]\nport = 443\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../Telemt/telemt-meko-fix.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	script = strings.ReplaceAll(script, "/run/lock/x-ui-telemt-meko.lock", filepath.Join(dir, "lock"))
	script = strings.ReplaceAll(script, "/etc/x-ui/telemt-meko-fix.env", filepath.Join(dir, "state.env"))
	script = strings.ReplaceAll(script, "/usr/local/x-ui/bin/mtproto/telemt-*.toml", filepath.Join(configDir, "telemt-*.toml"))
	script = strings.ReplaceAll(script, "/etc/x-ui/telemt.toml", legacy)
	helper := filepath.Join(dir, "meko.sh")
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, tool := range []string{"iptables", "nft"} {
		if err := os.WriteFile(filepath.Join(dir, tool), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	status := func() string {
		t.Helper()
		cmd := exec.Command("bash", helper, "status")
		cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"))
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("MEKO status: %v: %s", err, out)
		}
		return string(out)
	}
	if output := status(); !strings.Contains(output, "ports=7443,8443") || strings.Contains(output, "9443") || strings.Contains(output, "443,") {
		t.Fatalf("wrong selected ports: %s", output)
	}
	writeConfig("telemt-1.toml", false, 8443)
	if output := status(); !strings.Contains(output, "ports=7443") || strings.Contains(output, "8443") {
		t.Fatalf("disabling one MEKO inbound must not affect another: %s", output)
	}
	writeConfig("telemt-3.toml", false, 7443)
	if output := status(); !strings.Contains(output, "ports=") || strings.Contains(output, "ports=7") {
		t.Fatalf("no opted-in inbounds should yield no MEKO ports: %s", output)
	}
}

func TestMekoRenderedConfigMarkerIsOptIn(t *testing.T) {
	inst := Instance{Id: 1, Port: 8443}
	if strings.Contains(renderConfig(inst, 9000, "token"), "# x-ui-meko-fix=1") {
		t.Fatal("MEKO marker appeared on an ordinary inbound without the switch")
	}
	inst.MekoFix = true
	if !strings.HasPrefix(renderConfig(inst, 9000, "token"), "# x-ui-meko-fix=1\n") {
		t.Fatal("MEKO-enabled inbound must write its selected-port marker")
	}
}
