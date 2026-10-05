package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type gatewayTestHost struct {
	sys                          map[string]string
	rules                        int
	foreignRule, route, firewall string
	enabled, listener            bool
	failures                     map[string]int
	mutations                    int
}

func newGatewayTestHost(t *testing.T) (*GatewayNetworkService, *gatewayTestHost, GatewayNetworkConfig) {
	t.Helper()
	dir := t.TempDir()
	paths := &gatewayNetworkPaths{filepath.Join(dir, "gateway.env"), filepath.Join(dir, "rp"), filepath.Join(dir, "rules"), filepath.Join(dir, "routing.service"), filepath.Join(dir, "firewall.service"), filepath.Join(dir, "restore")}
	h := &gatewayTestHost{sys: map[string]string{"net.ipv4.ip_forward": "0", "net/ipv4/conf/all/rp_filter": "2", "net/ipv4/conf/default/rp_filter": "1"}, listener: true, failures: map[string]int{}}
	ifaces := []net.Interface{{Name: "lo"}, {Name: "eth0"}}
	for _, iface := range ifaces {
		h.sys["net/ipv4/conf/"+iface.Name+"/rp_filter"] = "2"
	}
	s := &GatewayNetworkService{paths: paths, run: h.run, input: h.input, interfaces: func() ([]net.Interface, error) { return ifaces, nil }}
	return s, h, GatewayNetworkConfig{LANInterface: "lo", LANIP: "192.168.50.1", LANPrefix: 24}
}
func (h *gatewayTestHost) fail(key string) error {
	if h.failures[key] > 0 {
		h.failures[key]--
		return fmt.Errorf("injected failure: %s", key)
	}
	return nil
}
func (h *gatewayTestHost) run(ctx context.Context, name string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	a := strings.Join(args, " ")
	switch name {
	case "ss":
		if h.listener {
			return "socket\n", nil
		}
		return "", nil
	case "sysctl":
		if args[0] == "-n" {
			v, ok := h.sys[args[1]]
			if !ok {
				return "", fmt.Errorf("unknown sysctl %s", args[1])
			}
			return v, nil
		}
		if err := h.fail("sysctl " + args[1]); err != nil {
			return "", err
		}
		key, value, _ := strings.Cut(args[1], "=")
		h.mutations++
		if key == "net.ipv4.ip_forward" && h.sys[key] != value {
			for k := range h.sys {
				if strings.HasSuffix(k, "/rp_filter") {
					h.sys[k] = "0"
				}
			}
		}
		h.sys[key] = value
		return "", nil
	case "ip":
		switch {
		case strings.Contains(a, "rule show"):
			out := h.foreignRule
			for i := 0; i < h.rules; i++ {
				out += fmt.Sprintf("%d: from all fwmark 0x40/0xc0 lookup 100\n", 32765-i)
			}
			return out, nil
		case strings.Contains(a, "route show"):
			return h.route, nil
		case strings.Contains(a, "rule add"):
			h.rules++
			h.mutations++
			return "", nil
		case strings.Contains(a, "rule del"):
			if h.rules == 0 {
				return "", errors.New("rule missing")
			}
			h.rules--
			h.mutations++
			return "", nil
		case strings.Contains(a, "route replace"):
			h.route = "local default dev lo scope host\n"
			h.mutations++
			return "", nil
		case strings.Contains(a, "route del"):
			h.route = ""
			h.mutations++
			return "", nil
		}
	case "nft":
		if a == "list tables" {
			out := ""
			if strings.Contains(h.firewall, "table inet xui_gateway {") {
				out += "table inet xui_gateway\n"
			}
			if strings.Contains(h.firewall, "table ip xui_gateway_nat {") {
				out += "table ip xui_gateway_nat\n"
			}
			return out, nil
		}
		if h.firewall != "" {
			return h.firewall, nil
		}
		return "", errors.New("table missing")
	case "systemctl":
		if args[0] == "is-enabled" {
			if h.enabled {
				return "enabled", nil
			}
			return "", errors.New("disabled")
		}
		if err := h.fail("systemctl " + args[0]); err != nil {
			return "", err
		}
		if args[0] == "enable" {
			h.enabled = true
		}
		if args[0] == "disable" {
			h.enabled = false
		}
		h.mutations++
		return "", nil
	}
	return "", fmt.Errorf("unhandled command: %s %s", name, a)
}
func (h *gatewayTestHost) input(ctx context.Context, input, name string, args ...string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := "nft apply"
	if strings.Contains(strings.Join(args, " "), "-c") {
		key = "nft check"
	}
	if err := h.fail(key); err != nil {
		return err
	}
	if key == "nft check" {
		return nil
	}
	h.mutations++
	if strings.Contains(input, "table inet xui_gateway {") {
		h.firewall = input
	} else {
		h.firewall = ""
	}
	return nil
}
func assertGatewayHealthy(t *testing.T, s *GatewayNetworkService) {
	t.Helper()
	status := s.Status(context.Background())
	if !status.Configured || !status.Forwarding || !status.RPFilter || !status.PolicyRoute || !status.NFTables || !status.NAT || !status.Persistent || !status.ListenerReady || status.Error != "" {
		t.Fatalf("unhealthy: %+v", status)
	}
}
func TestGatewayNetworkLifecyclePreservesBaseline(t *testing.T) {
	s, h, cfg := newGatewayTestHost(t)
	baseline := map[string]string{}
	for k, v := range h.sys {
		baseline[k] = v
	}
	ctx := context.Background()
	if err := s.Enable(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	assertGatewayHealthy(t, s)
	snapshot, err := os.ReadFile(s.paths.rpFilter)
	if err != nil {
		t.Fatal(err)
	}
	h.rules = 2 // Repair duplicate rules left by the legacy CLI.
	if err := s.Enable(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.paths.rpFilter)
	if string(after) != string(snapshot) || h.rules != 1 {
		t.Fatal("repair overwrote baseline or duplicated policy rules")
	}
	assertGatewayHealthy(t, s)
	if err := s.Suspend(ctx); err != nil {
		t.Fatal(err)
	}
	state, _ := s.loadGatewayNetworkState()
	if state.DesiredActive || h.firewall != "" {
		t.Fatal("suspended Gateway still intercepts or restores at boot")
	}
	if err := s.Enable(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	assertGatewayHealthy(t, s)
	if err := s.Disable(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(h.sys, baseline) || h.rules != 0 || h.route != "" || h.firewall != "" || h.enabled {
		t.Fatalf("baseline not restored: %+v", h)
	}
	if _, err := os.Stat(s.paths.state); !os.IsNotExist(err) {
		t.Fatalf("recovery state survived successful disable: %v", err)
	}
	if err := s.Disable(ctx); err != nil {
		t.Fatal("repeated disable:", err)
	}
}
func TestGatewayNetworkFailureRollback(t *testing.T) {
	for _, stage := range []string{"nft check", "sysctl net.ipv4.ip_forward=1", "nft apply", "systemctl daemon-reload", "systemctl enable"} {
		t.Run(stage, func(t *testing.T) {
			s, h, cfg := newGatewayTestHost(t)
			baseline := map[string]string{}
			for k, v := range h.sys {
				baseline[k] = v
			}
			h.failures[stage] = 1
			if err := s.Enable(context.Background(), cfg); err == nil {
				t.Fatal("fault ignored")
			}
			if !reflect.DeepEqual(h.sys, baseline) || h.firewall != "" || h.rules != 0 || h.route != "" {
				t.Fatalf("failed setup leaked changes: %+v", h)
			}
			if _, err := os.Stat(s.paths.state); !os.IsNotExist(err) {
				t.Fatalf("rollback did not clear state: %v", err)
			}
		})
	}
}
func TestGatewayFailedRollbackRetainsRecovery(t *testing.T) {
	s, h, cfg := newGatewayTestHost(t)
	h.failures["systemctl enable"] = 1
	h.failures["systemctl disable"] = 1
	if err := s.Enable(context.Background(), cfg); err == nil || !strings.Contains(err.Error(), "rollback failed") {
		t.Fatalf("error=%v", err)
	}
	state, err := s.loadGatewayNetworkState()
	if err != nil || state.DesiredActive {
		t.Fatalf("unsafe recovery marker: %+v %v", state, err)
	}
	if _, err := os.Stat(s.paths.rpFilter); err != nil {
		t.Fatal("lost recovery snapshot:", err)
	}
	if h.firewall != "" {
		t.Fatal("failed cleanup left interception active")
	}
	if err := s.Disable(context.Background()); err != nil {
		t.Fatal("retry cleanup:", err)
	}
}
func TestGatewayRepairFailureKeepsExistingFirewall(t *testing.T) {
	s, h, cfg := newGatewayTestHost(t)
	if err := s.Enable(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	previous := h.firewall
	h.failures["nft apply"] = 1
	if err := s.Enable(context.Background(), cfg); err == nil {
		t.Fatal("repair failure ignored")
	}
	if h.firewall != previous {
		t.Fatal("repair removed old firewall before replacement validated/applied")
	}
	if _, err := os.Stat(s.paths.rpFilter); err != nil {
		t.Fatal("repair lost baseline")
	}
}
func TestGatewayRepairLateFailureSuspendsInterception(t *testing.T) {
	s, h, cfg := newGatewayTestHost(t)
	if err := s.Enable(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	h.failures["systemctl enable"] = 1
	if err := s.Enable(context.Background(), cfg); err == nil {
		t.Fatal("late repair failure ignored")
	}
	state, err := s.loadGatewayNetworkState()
	if err != nil {
		t.Fatal(err)
	}
	if state.DesiredActive {
		t.Fatal("failed repair remained armed for boot restoration")
	}
	if h.firewall != "" {
		t.Fatal("failed repair left TPROXY interception active")
	}
	if err := s.Enable(context.Background(), cfg); err != nil {
		t.Fatal("repair retry failed:", err)
	}
	assertGatewayHealthy(t, s)
}
func TestGatewayMissingRPFilterSnapshotFailsClosed(t *testing.T) {
	s, h, cfg := newGatewayTestHost(t)
	baseline := map[string]string{}
	for key, value := range h.sys {
		baseline[key] = value
	}
	if err := s.Enable(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	snapshot, err := os.ReadFile(s.paths.rpFilter)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(s.paths.rpFilter); err != nil {
		t.Fatal(err)
	}
	if status := s.Status(context.Background()); status.Error == "" {
		t.Fatal("missing recovery baseline was not reported")
	}
	if err := s.Enable(context.Background(), cfg); err == nil {
		t.Fatal("repair recreated a missing recovery baseline from modified sysctls")
	}
	if h.firewall == "" {
		t.Fatal("rejected repair unnecessarily removed the previously active firewall")
	}
	if err := s.Suspend(context.Background()); err == nil {
		t.Fatal("suspend hid the missing recovery baseline")
	}
	if h.firewall != "" {
		t.Fatal("baseline failure did not fail closed")
	}
	state, err := s.loadGatewayNetworkState()
	if err != nil || state.DesiredActive {
		t.Fatalf("unsafe suspended state: %+v %v", state, err)
	}
	if err := os.WriteFile(s.paths.rpFilter, snapshot, 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Disable(context.Background()); err != nil {
		t.Fatal("cleanup after restoring baseline:", err)
	}
	if !reflect.DeepEqual(h.sys, baseline) {
		t.Fatalf("restored baseline differs: got=%v want=%v", h.sys, baseline)
	}
}

func TestGatewayLegacySuspendCapturesBaselineAndDisablesOldBootUnits(t *testing.T) {
	s, h, cfg := newGatewayTestHost(t)
	legacy := fmt.Sprintf("IP_FORWARD_OLD=0\nLAN_IF=%s\nLAN_IP=%s\nLAN_PREFIX=%d\nLAN_NETWORK=192.168.50.0/24\nWAN_IF=\n", cfg.LANInterface, cfg.LANIP, cfg.LANPrefix)
	if err := os.WriteFile(s.paths.state, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{s.paths.routingUnit, s.paths.firewallUnit} {
		if err := os.WriteFile(path, []byte("legacy"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	h.enabled = true
	h.firewall = gatewayNFTRules("192.168.50.0/24", "", cfg.LANInterface)
	if err := s.Suspend(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.firewall != "" || h.enabled {
		t.Fatalf("legacy suspension left runtime/boot interception active: %+v", h)
	}
	if _, err := os.Stat(s.paths.rpFilter); err != nil {
		t.Fatal("legacy baseline was not captured:", err)
	}
	state, err := s.loadGatewayNetworkState()
	if err != nil || state.Legacy || state.DesiredActive {
		t.Fatalf("legacy state was not migrated safely: %+v %v", state, err)
	}
}
func TestGatewayRejectsForeignNetworkOwnership(t *testing.T) {
	for _, kind := range []string{"rule", "route", "firewall"} {
		t.Run(kind, func(t *testing.T) {
			s, h, cfg := newGatewayTestHost(t)
			switch kind {
			case "rule":
				h.foreignRule = "100: from all lookup 100\n"
			case "route":
				h.route = "default via 192.168.1.1 dev eth0\n"
			case "firewall":
				h.firewall = "table inet xui_gateway {}"
			}
			if err := s.Enable(context.Background(), cfg); err == nil {
				t.Fatal("foreign state overwritten")
			}
			if h.mutations != 0 {
				t.Fatal("ownership rejection mutated host")
			}
		})
	}
}
func TestGatewayListenerFailureHasNoSideEffects(t *testing.T) {
	s, h, cfg := newGatewayTestHost(t)
	h.listener = false
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := s.Enable(ctx, cfg); err == nil {
		t.Fatal("setup succeeded without listener")
	}
	if h.mutations != 0 {
		t.Fatal("kernel changed without listener")
	}
}
func TestGatewayInvalidSnapshotsAndHealth(t *testing.T) {
	for _, input := range []string{"", "net/ipv4/conf/all/rp_filter 3", "net/ipv4/conf/../../rp_filter 0", "net/ipv4/conf/all/rp_filter 1\nnet/ipv4/conf/all/rp_filter 0"} {
		if _, err := parseGatewayRPSnapshot(input); err == nil {
			t.Fatalf("invalid snapshot accepted: %q", input)
		}
	}
	s, h, cfg := newGatewayTestHost(t)
	if err := s.Enable(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	h.enabled = false
	h.listener = false
	status := s.Status(context.Background())
	if status.Persistent || status.ListenerReady {
		t.Fatal("status masks missing persistence/listener")
	}
	if err := os.WriteFile(s.paths.state, []byte("IP_FORWARD_OLD=1\nLAN_IF=lo\nLAN_IF=evil\n"), 0600); err != nil {
		t.Fatal(err)
	}
	status = s.Status(context.Background())
	if !status.Configured || status.Error == "" {
		t.Fatal("corrupt state invisible")
	}
}
func TestGatewayRestoreScriptSyntaxAndOrdering(t *testing.T) {
	s, _, _ := newGatewayTestHost(t)
	routing, firewall, script, err := s.gatewayNetworkUnits()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-n")
	cmd.Stdin = strings.NewReader(script)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("invalid script: %v %s", err, output)
	}
	if !strings.Contains(firewall, "After=network-online.target nftables.service x-ui.service xui-gateway-routing.service") || strings.Contains(routing, "ExecStop=") {
		t.Fatal("unsafe service ordering/stop semantics")
	}
	if strings.Contains(script, "@STATE@") || !strings.Contains(script, "NETWORK_ENABLED=0") || !strings.Contains(script, "flock") {
		t.Fatal("recovery script missing state guard/lock")
	}
}

func TestGatewayBootRestoreRunsIdempotentTransactions(t *testing.T) {
	s, _, cfg := newGatewayTestHost(t)
	dir := t.TempDir()
	logPath := filepath.Join(dir, "calls")
	rulesPath := filepath.Join(dir, "policy")
	scriptTool := `#!/bin/bash
set -eu
name=${0##*/}
printf '%s %s\n' "$name" "$*" >> ` + gatewayShellQuote(logPath) + `
case "$name $*" in
 'ip -N -4 rule show') cat ` + gatewayShellQuote(rulesPath) + ` ;;
 'ip -4 route show table 100') : ;;
 'nft list tables') printf 'table inet xui_gateway\ntable ip xui_gateway_nat\n' ;;
 'nft -c -f '*|'nft -f '*) cat "${@: -1}" >> ` + gatewayShellQuote(logPath) + ` ;;
 'ss '*) printf 'socket\n' ;;
 'sysctl -n '*) printf '1\n' ;;
esac
`
	for _, name := range []string{"ip", "nft", "ss", "sysctl"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(scriptTool), 0700); err != nil {
			t.Fatal(err)
		}
	}
	_, _, script, err := s.gatewayNetworkUnits()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"ip", "nft", "ss", "sysctl"} {
		script = strings.ReplaceAll(script, gatewayShellQuote("/usr/bin/"+name), gatewayShellQuote(filepath.Join(dir, name)))
	}
	scriptPath := filepath.Join(dir, "restore")
	if err := os.WriteFile(scriptPath, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := s.saveGatewayNetworkState(gatewayNetworkState{OldForwarding: "0", Config: cfg, LANNetwork: "192.168.50.0/24", DesiredActive: true}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.paths.nft, []byte(gatewayNFTRules("192.168.50.0/24", "", "lo")), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.paths.rpFilter, []byte("net/ipv4/conf/all/rp_filter 2\nnet/ipv4/conf/lo/rp_filter 2\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(rulesPath, []byte("32764: from all fwmark 0x40/0xc0 lookup 100\n32765: from all fwmark 0x40/0xc0 lookup 100\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run := func(mode string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, "bash", scriptPath, mode).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, output)
		}
		return nil
	}
	if err := run("routing"); err != nil {
		t.Fatal(err)
	}
	if err := run("firewall"); err != nil {
		t.Fatal(err)
	}
	calls, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(calls)
	if strings.Count(log, "ip -4 rule del priority ") != 2 || strings.Count(log, "ip -4 rule add fwmark") != 1 {
		t.Fatalf("boot restore did not deduplicate owned rules: %s", log)
	}
	check := strings.Index(log, "nft -c -f")
	apply := strings.Index(log, "nft -f")
	if check < 0 || apply <= check || !strings.Contains(log, "delete table inet xui_gateway") {
		t.Fatalf("boot restore skipped atomic replacement validation: %s", log)
	}
	if err := os.WriteFile(rulesPath, []byte("100: from all fwmark 0x40/0xc0 lookup 100 uidrange 1000-1000\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run("routing"); err == nil {
		t.Fatal("boot restore accepted a foreign policy selector")
	}
	state, _ := s.loadGatewayNetworkState()
	state.DesiredActive = false
	if err := s.saveGatewayNetworkState(state); err != nil {
		t.Fatal(err)
	}
	previous, _ := os.ReadFile(logPath)
	if err := run("routing"); err != nil {
		t.Fatal(err)
	}
	if err := run("firewall"); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(logPath)
	if string(after) != string(previous) {
		t.Fatal("inactive recovery marker executed network commands at boot")
	}
}
