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
 '-f -') cat > "$MEKO_TEST_DIR/batch"; touch "$MEKO_TEST_DIR/nft-installed" ;;
 'list table inet xui_telemt_meko') test -e "$MEKO_TEST_DIR/nft-installed" ;;
 'list chain inet xui_telemt_meko input') echo 'reject with tcp reset' ;;
 '-n list set inet xui_telemt_meko ports') printf 'elements = { 443,\n 8443 }\n' ;;
 'delete table inet xui_telemt_meko') touch "$MEKO_TEST_DIR/deleted"; rm -f "$MEKO_TEST_DIR/nft-installed" ;;
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
	for _, want := range []string{"add table inet xui_telemt_meko", "ports { 443,8443 }", "@th,320,32 0x04020000", "@th,160,16 0x0204", "ip saddr timeout 60s", "ip6 saddr timeout 60s", "54/minute burst 1 packets", "counter return"} {
		if !strings.Contains(string(batch), want) {
			t.Errorf("missing %s: %s", want, batch)
		}
	}
	if strings.Count(string(batch), "reject with tcp reset") != 4 {
		t.Fatal("expected one IPv4 and IPv6 limiter per port")
	}
	if strings.Contains(string(batch), "delete table") {
		t.Fatal("first apply should not delete a missing nft table")
	}
	// Reapplying with an existing named set must replace the owned table
	// atomically, without a period of unprotected firewall state.
	run("apply", "1")
	batch, err = os.ReadFile(filepath.Join(dir, "batch"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(batch), "delete table inet xui_telemt_meko\\nadd table inet xui_telemt_meko") {
		t.Fatalf("subsequent apply must replace nft table atomically: %s", batch)
	}
	if strings.Contains(string(batch), "flush table") {
		t.Fatal("flush table retains named sets and breaks their re-creation")
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
		cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"), "TELEMT_MEKO_ENABLED=0")
		// Simulate the legacy systemd EnvironmentFile setting MEKO=0.
		// Explicitly opted-in inbounds must still take precedence.
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("MEKO status: %v: %s", err, out)
		}
		return string(out)
	}
	selected := func() string {
		for _, line := range strings.Split(status(), "\n") {
			if strings.HasPrefix(line, "ports=") {
				return strings.TrimPrefix(line, "ports=")
			}
		}
		return "missing ports field"
	}
	if ports := selected(); ports != "7443,8443" {
		t.Fatalf("wrong selected ordinary ports: %q", ports)
	}
	if out := status(); !strings.Contains(out, "enabled=true") {
		t.Fatalf("old globally disabled MEKO state blocked per-inbound selection: %s", out)
	}
	writeConfig("telemt-1.toml", false, 8443)
	if ports := selected(); ports != "7443" {
		t.Fatalf("disabling one MEKO inbound must not affect another: %q", ports)
	}
	writeConfig("telemt-3.toml", false, 7443)
	if ports := selected(); ports != "" {
		t.Fatalf("no opted-in inbounds should yield no MEKO ports: %q", ports)
	}
	if out := status(); !strings.Contains(out, "enabled=false") {
		t.Fatalf("MEKO remained enabled with no selected ports: %s", out)
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

func TestMekoIndependentInboundLimits(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux helper")
	}
	if _, err := exec.LookPath("flock"); err != nil {
		t.Skip("flock unavailable")
	}
	dir := t.TempDir()
	configs := filepath.Join(dir, "mtproto")
	if err := os.Mkdir(configs, 0o700); err != nil {
		t.Fatal(err)
	}
	write := func(name string, selected bool, port, rate, burst int) {
		t.Helper()
		body := ""
		if selected {
			body = fmt.Sprintf("# x-ui-meko-fix=1\n# x-ui-meko-rate=%d\n# x-ui-meko-burst=%d\n", rate, burst)
		}
		body += fmt.Sprintf("[server]\nport = %d\n", port)
		if err := os.WriteFile(filepath.Join(configs, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("telemt-1.toml", true, 8443, 120, 8)
	write("telemt-2.toml", true, 7443, 54, 1)
	write("telemt-3.toml", false, 9443, 0, 0)

	raw, err := os.ReadFile("../Telemt/telemt-meko-fix.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := strings.ReplaceAll(string(raw), "/run/lock/x-ui-telemt-meko.lock", filepath.Join(dir, "lock"))
	script = strings.ReplaceAll(script, "/etc/x-ui/telemt-meko-fix.env", filepath.Join(dir, "state.env"))
	script = strings.ReplaceAll(script, "/usr/local/x-ui/bin/mtproto/telemt-*.toml", filepath.Join(configs, "telemt-*.toml"))
	helper := filepath.Join(dir, "helper.sh")
	if err := os.WriteFile(helper, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	mock := "#!/usr/bin/env bash\nset -eu\nif [[ \"$*\" == \"-f -\" ]]; then cat > \"$MEKO_TEST_DIR/batch\"; exit 0; fi\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "nft"), []byte(mock), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "iptables"), []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	apply := func() string {
		t.Helper()
		cmd := exec.Command("bash", helper, "apply")
		cmd.Env = append(os.Environ(), "PATH="+dir+string(os.PathListSeparator)+os.Getenv("PATH"),
			"MEKO_TEST_DIR="+dir, "TELEMT_MEKO_ENABLED=0", "TELEMT_MEKO_RATE=54/minute", "TELEMT_MEKO_BURST=1")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("apply: %v: %s", err, out)
		}
		batch, err := os.ReadFile(filepath.Join(dir, "batch"))
		if err != nil {
			t.Fatal(err)
		}
		return string(batch)
	}
	batch := apply()
	for _, want := range []string{"ports { 7443,8443 }", "120/minute burst 8 packets", "54/minute burst 1 packets"} {
		if !strings.Contains(batch, want) {
			t.Errorf("missing %q: %s", want, batch)
		}
	}
	if strings.Contains(batch, "tcp dport 9443") {
		t.Fatal("disabled inbound acquired rules")
	}
	write("telemt-1.toml", true, 8443, 240, 4)
	batch = apply()
	if !strings.Contains(batch, "240/minute burst 4 packets") || strings.Contains(batch, "120/minute burst 8 packets") {
		t.Fatal("changed per-port rate was not applied")
	}
	write("telemt-1.toml", false, 8443, 0, 0)
	batch = apply()
	if !strings.Contains(batch, "ports { 7443 }") || strings.Contains(batch, "tcp dport 8443") {
		t.Fatal("disabling one MEKO inbound should remove only its port")
	}
}

func TestMekoPolicyEditsDoNotRestartTelemt(t *testing.T) {
	base := Instance{Id: 7, Port: 8443, Secrets: []SecretEntry{{Name: "a", Secret: "0123456789abcdef0123456789abcdef"}}}
	modified := base
	modified.MekoFix = true
	modified.MekoRatePerMinute = 120
	modified.MekoBurst = 8
	if base.structuralFingerprint() != modified.structuralFingerprint() {
		t.Fatal("changing only MEKO firewall policy must not restart Telemt")
	}
	if base.mekoFingerprint() == modified.mekoFingerprint() {
		t.Fatal("MEKO fingerprint must change when the per-port policy changes")
	}
	if base.secretsFingerprint() != modified.secretsFingerprint() {
		t.Fatal("MEKO changes must not alter users")
	}
	config := renderConfig(modified, 9000, "token")
	for _, marker := range []string{"# x-ui-meko-fix=1\n", "# x-ui-meko-rate=120\n", "# x-ui-meko-burst=8\n"} {
		if !strings.Contains(config, marker) {
			t.Fatalf("missing selected port marker %q", marker)
		}
	}
	off := renderConfig(base, 9000, "token")
	if strings.Contains(off, "# x-ui-meko-") {
		t.Fatal("MEKO metadata must never appear in unselected proxy configs")
	}
}
