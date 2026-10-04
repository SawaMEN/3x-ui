package gateway

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	gatewayStatePath       = "/etc/x-ui/gateway.env"
	gatewayRulesPath       = "/etc/x-ui/gateway.nft"
	gatewayRoutingUnitPath = "/etc/systemd/system/xui-gateway-routing.service"
	gatewayFirewallUnit    = "/etc/systemd/system/xui-gateway-firewall.service"
	gatewayRestoreScript   = "/usr/local/sbin/xui-gateway-firewall-restore"
	gatewayRoutingUnit     = "xui-gateway-routing.service"
	gatewayFirewallService = "xui-gateway-firewall.service"
)

var gatewayInterfaceName = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,15}$`)

// SystemConfig describes the Linux side of Gateway Mode. LANIP is the address
// assigned to LANInterface; LANPrefix is used to derive the source network.
type SystemConfig struct {
	LANInterface string `json:"lanInterface"`
	LANIP        string `json:"lanIP"`
	LANPrefix    int    `json:"lanPrefix"`
	WANInterface string `json:"wanInterface,omitempty"`
}

// SystemState reports both the persisted Gateway settings and whether the
// corresponding live Linux forwarding/routing/firewall state is healthy.
type SystemState struct {
	Configured   bool
	Active       bool
	IPForward    bool
	PolicyRoute  bool
	Firewall     bool
	NAT          bool
	Config       SystemConfig
	LANNetwork   string
	OldIPForward string
}

func normalizeSystemConfig(input SystemConfig) (SystemConfig, string, error) {
	cfg := SystemConfig{
		LANInterface: strings.TrimSpace(input.LANInterface),
		LANIP:        strings.TrimSpace(input.LANIP),
		LANPrefix:    input.LANPrefix,
		WANInterface: strings.TrimSpace(input.WANInterface),
	}
	if !gatewayInterfaceName.MatchString(cfg.LANInterface) {
		return SystemConfig{}, "", fmt.Errorf("invalid LAN interface %q", cfg.LANInterface)
	}
	lan, err := net.InterfaceByName(cfg.LANInterface)
	if err != nil {
		return SystemConfig{}, "", fmt.Errorf("LAN interface %q does not exist: %w", cfg.LANInterface, err)
	}
	addr, err := netip.ParseAddr(cfg.LANIP)
	if err != nil || !addr.Is4() {
		return SystemConfig{}, "", fmt.Errorf("invalid LAN IPv4 address %q", cfg.LANIP)
	}
	if cfg.LANPrefix < 0 || cfg.LANPrefix > 32 {
		return SystemConfig{}, "", fmt.Errorf("LAN prefix must be between 0 and 32")
	}
	assigned := false
	if addrs, addrErr := lan.Addrs(); addrErr == nil {
		for _, item := range addrs {
			value := strings.SplitN(item.String(), "/", 2)[0]
			candidate, parseErr := netip.ParseAddr(value)
			if parseErr == nil && candidate.Unmap() == addr.Unmap() {
				assigned = true
				break
			}
		}
	} else {
		return SystemConfig{}, "", fmt.Errorf("read addresses for LAN interface %q: %w", cfg.LANInterface, addrErr)
	}
	if !assigned {
		return SystemConfig{}, "", fmt.Errorf("LAN IPv4 address %s is not assigned to interface %s", cfg.LANIP, cfg.LANInterface)
	}
	if cfg.WANInterface != "" {
		if !gatewayInterfaceName.MatchString(cfg.WANInterface) {
			return SystemConfig{}, "", fmt.Errorf("invalid WAN interface %q", cfg.WANInterface)
		}
		if cfg.WANInterface == cfg.LANInterface {
			return SystemConfig{}, "", fmt.Errorf("LAN and WAN interfaces must be different")
		}
		if _, err := net.InterfaceByName(cfg.WANInterface); err != nil {
			return SystemConfig{}, "", fmt.Errorf("WAN interface %q does not exist: %w", cfg.WANInterface, err)
		}
	}
	return cfg, netip.PrefixFrom(addr, cfg.LANPrefix).Masked().String(), nil
}

// ValidateSystemConfig validates and normalizes a web/CLI supplied Linux
// Gateway configuration without changing the host.
func ValidateSystemConfig(input SystemConfig) (SystemConfig, error) {
	cfg, _, err := normalizeSystemConfig(input)
	return cfg, err
}

func sameSystemConfig(a, b SystemConfig) bool {
	return a.LANInterface == b.LANInterface && a.LANIP == b.LANIP && a.LANPrefix == b.LANPrefix && a.WANInterface == b.WANInterface
}

func parseGatewayState(data []byte) (SystemState, error) {
	state := SystemState{Configured: true, NAT: true}
	values := make(map[string]string)
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return state, fmt.Errorf("invalid Gateway state line %q", line)
		}
		values[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	prefix, err := strconv.Atoi(values["LAN_PREFIX"])
	if err != nil || prefix < 0 || prefix > 32 {
		return state, fmt.Errorf("invalid LAN_PREFIX in %s", gatewayStatePath)
	}
	state.Config = SystemConfig{
		LANInterface: values["LAN_IF"],
		LANIP:        values["LAN_IP"],
		LANPrefix:    prefix,
		WANInterface: values["WAN_IF"],
	}
	state.LANNetwork = values["LAN_NETWORK"]
	state.OldIPForward = values["IP_FORWARD_OLD"]
	if state.Config.LANInterface == "" || state.Config.LANIP == "" || state.LANNetwork == "" {
		return state, fmt.Errorf("incomplete Gateway state in %s", gatewayStatePath)
	}
	if state.OldIPForward != "0" && state.OldIPForward != "1" {
		return state, fmt.Errorf("invalid IP_FORWARD_OLD in %s", gatewayStatePath)
	}
	return state, nil
}

func readGatewayState() (SystemState, error) {
	data, err := os.ReadFile(gatewayStatePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SystemState{}, nil
		}
		return SystemState{}, fmt.Errorf("read Gateway system state: %w", err)
	}
	return parseGatewayState(data)
}

func command(name string, args ...string) ([]byte, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return nil, fmt.Errorf("required command %q not found: %w", name, err)
	}
	out, err := exec.Command(path, args...).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text != "" {
			return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, text)
		}
		return out, fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return out, nil
}

func commandInput(input, name string, args ...string) error {
	path, err := exec.LookPath(name)
	if err != nil {
		return fmt.Errorf("required command %q not found: %w", name, err)
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text != "" {
			return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, text)
		}
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func commandIgnore(name string, args ...string) {
	path, err := exec.LookPath(name)
	if err != nil {
		return
	}
	_ = exec.Command(path, args...).Run()
}

func commandPath(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("required command %q not found: %w", name, err)
	}
	return path, nil
}

func writeAtomic(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".xui-gateway-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func writeGatewayState(cfg SystemConfig, lanNetwork, oldForward string) error {
	data := fmt.Sprintf("IP_FORWARD_OLD=%s\nLAN_IF=%s\nLAN_IP=%s\nLAN_PREFIX=%d\nLAN_NETWORK=%s\nWAN_IF=%s\n",
		oldForward, cfg.LANInterface, cfg.LANIP, cfg.LANPrefix, lanNetwork, cfg.WANInterface)
	if err := writeAtomic(gatewayStatePath, []byte(data), 0o600); err != nil {
		return fmt.Errorf("persist Gateway system state: %w", err)
	}
	return nil
}

func nftGatewayRules(lanNetwork string) string {
	return fmt.Sprintf(`table inet xui_gateway {
    set whitelist {
        type ipv4_addr
        flags interval
        auto-merge
        elements = { 0.0.0.0/8, 10.0.0.0/8, 100.64.0.0/10, 127.0.0.0/8, 169.254.0.0/16, 172.16.0.0/12, 192.0.0.0/24, 192.0.2.0/24, 192.88.99.0/24, 192.168.0.0/16, 198.51.100.0/24, 203.0.113.0/24, 224.0.0.0/3 }
    }
    set whitelist6 {
        type ipv6_addr
        flags interval
        auto-merge
        elements = { ::/127, fc00::/7, fe80::/10, ff00::/8 }
    }
    set interface {
        type ipv4_addr
        flags interval
        auto-merge
        elements = { 127.0.0.0/8, %s }
    }
    set interface6 {
        type ipv6_addr
        flags interval
        auto-merge
        elements = { ::1, fe80::/64 }
    }
    chain tp_out {
        meta mark & 0x00000080 == 0x00000080 return
        meta l4proto { tcp, udp } fib saddr type local fib daddr type != local jump tp_rule
    }
    chain tp_pre {
        iifname "lo" meta mark & 0x000000c0 != 0x00000040 return
        meta l4proto { tcp, udp } fib saddr type != local fib daddr type != local jump tp_rule
        meta l4proto { tcp, udp } meta mark & 0x000000c0 == 0x00000040 tproxy ip to 127.0.0.1:52345
        meta l4proto { tcp, udp } meta mark & 0x000000c0 == 0x00000040 tproxy ip6 to [::1]:52345
    }
    chain output {
        type route hook output priority mangle - 5;
        policy accept;
    }
    chain prerouting {
        type filter hook prerouting priority mangle - 5;
        policy accept;
        meta nfproto { ipv4, ipv6 } jump tp_pre
    }
    chain tp_rule {
        meta mark set ct mark
        meta mark & 0x000000c0 == 0x00000040 return
        iifname "br-*" return
        iifname "docker*" return
        iifname "veth*" return
        iifname "wg*" return
        iifname "ppp*" return
        ip daddr @interface return
        ip daddr @whitelist return
        ip6 daddr @whitelist6 return
        ip6 daddr @interface6 return
        jump tp_mark
    }
    chain tp_mark {
        tcp flags syn / fin,syn,rst,ack meta mark set meta mark | 0x00000040
        meta l4proto udp ct state new meta mark set meta mark | 0x00000040
        ct mark set meta mark
    }
}
`, lanNetwork)
}

func nftNATRules(lanNetwork, wanInterface string) string {
	return fmt.Sprintf(`table ip xui_gateway_nat {
    chain postrouting {
        type nat hook postrouting priority srcnat;
        policy accept;
        ip saddr %s oifname "%s" masquerade
    }
}
`, lanNetwork, wanInterface)
}

func deleteGatewayNFT() {
	commandIgnore("nft", "delete", "table", "inet", "xui_gateway")
	commandIgnore("nft", "delete", "table", "ip", "xui_gateway_nat")
}

func applyGatewayNFT(lanNetwork, wanInterface string) error {
	deleteGatewayNFT()
	if err := commandInput(nftGatewayRules(lanNetwork), "nft", "-f", "-"); err != nil {
		return fmt.Errorf("apply Gateway nftables rules: %w", err)
	}
	if wanInterface != "" {
		if err := commandInput(nftNATRules(lanNetwork, wanInterface), "nft", "-f", "-"); err != nil {
			deleteGatewayNFT()
			return fmt.Errorf("apply Gateway NAT rules: %w", err)
		}
	}
	return nil
}

func clearPolicyRouting() {
	for i := 0; i < 8; i++ {
		if _, err := command("ip", "rule", "del", "fwmark", "0x40/0xc0", "table", "100"); err != nil {
			break
		}
	}
	for i := 0; i < 8; i++ {
		if _, err := command("ip", "route", "del", "local", "default", "dev", "lo", "table", "100"); err != nil {
			break
		}
	}
}

func enablePolicyRouting() error {
	ipPath, err := commandPath("ip")
	if err != nil {
		return err
	}
	commandIgnore("systemctl", "disable", "--now", gatewayRoutingUnit)
	clearPolicyRouting()
	unit := fmt.Sprintf(`[Unit]
Description=3X-UI Gateway Mode Policy Routing
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=%s rule add fwmark 0x40/0xc0 table 100
ExecStart=%s route add local default dev lo table 100
RemainAfterExit=yes
ExecStop=%s rule del fwmark 0x40/0xc0 table 100
ExecStop=%s route del local default dev lo table 100

[Install]
WantedBy=multi-user.target
`, ipPath, ipPath, ipPath, ipPath)
	if err := writeAtomic(gatewayRoutingUnitPath, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("write Gateway routing service: %w", err)
	}
	if _, err := command("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if _, err := command("systemctl", "enable", "--now", gatewayRoutingUnit); err != nil {
		clearPolicyRouting()
		return fmt.Errorf("enable Gateway policy routing: %w", err)
	}
	return nil
}

func disablePolicyRouting() {
	commandIgnore("systemctl", "disable", "--now", gatewayRoutingUnit)
	_ = os.Remove(gatewayRoutingUnitPath)
	commandIgnore("systemctl", "daemon-reload")
	clearPolicyRouting()
}

func persistGatewayFirewall() error {
	mainRules, err := command("nft", "list", "table", "inet", "xui_gateway")
	if err != nil {
		return fmt.Errorf("save Gateway nftables table: %w", err)
	}
	var rules bytes.Buffer
	rules.Write(mainRules)
	if len(mainRules) > 0 && mainRules[len(mainRules)-1] != '\n' {
		rules.WriteByte('\n')
	}
	if natRules, natErr := command("nft", "list", "table", "ip", "xui_gateway_nat"); natErr == nil {
		rules.Write(natRules)
		if len(natRules) > 0 && natRules[len(natRules)-1] != '\n' {
			rules.WriteByte('\n')
		}
	}
	if err := writeAtomic(gatewayRulesPath, rules.Bytes(), 0o600); err != nil {
		return fmt.Errorf("write persisted Gateway nftables rules: %w", err)
	}

	nftPath, err := commandPath("nft")
	if err != nil {
		return err
	}
	sysctlPath, err := commandPath("sysctl")
	if err != nil {
		return err
	}
	restore := fmt.Sprintf(`#!/usr/bin/env bash
set -euo pipefail
STATE=%q
RULES=%q
[[ -f "${STATE}" ]] || exit 0
[[ -s "${RULES}" ]] || exit 0
%s -w net.ipv4.ip_forward=1
%s delete table inet xui_gateway 2>/dev/null || true
%s delete table ip xui_gateway_nat 2>/dev/null || true
%s -f "${RULES}"
`, gatewayStatePath, gatewayRulesPath, sysctlPath, nftPath, nftPath, nftPath)
	if err := writeAtomic(gatewayRestoreScript, []byte(restore), 0o700); err != nil {
		return fmt.Errorf("write Gateway firewall restore script: %w", err)
	}
	unit := `[Unit]
Description=3X-UI Gateway Mode Firewall Restore
Wants=network-online.target
After=network-online.target nftables.service

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/xui-gateway-firewall-restore
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`
	if err := writeAtomic(gatewayFirewallUnit, []byte(unit), 0o644); err != nil {
		return fmt.Errorf("write Gateway firewall service: %w", err)
	}
	if _, err := command("systemctl", "daemon-reload"); err != nil {
		return err
	}
	if _, err := command("systemctl", "enable", gatewayFirewallService); err != nil {
		return fmt.Errorf("enable Gateway firewall persistence: %w", err)
	}
	return nil
}

func disableFirewallPersistence() {
	commandIgnore("systemctl", "disable", "--now", gatewayFirewallService)
	_ = os.Remove(gatewayFirewallUnit)
	_ = os.Remove(gatewayRestoreScript)
	_ = os.Remove(gatewayRulesPath)
	_ = os.Remove(gatewayRulesPath + ".tmp")
	commandIgnore("systemctl", "daemon-reload")
}

func restoreIPForward(value string) error {
	if value != "0" && value != "1" {
		return nil
	}
	_, err := command("sysctl", "-w", "net.ipv4.ip_forward="+value)
	return err
}

func rollbackFreshSystem(oldForward string) {
	disableFirewallPersistence()
	deleteGatewayNFT()
	disablePolicyRouting()
	_ = restoreIPForward(oldForward)
	_ = os.Remove(gatewayStatePath)
}

// EnableSystem applies or repairs the Linux side of Gateway Mode and persists
// it across reboot. It intentionally uses the same files/tables/units as the
// existing x-ui shell Gateway implementation for full CLI/web compatibility.
func EnableSystem(input SystemConfig) error {
	cfg, lanNetwork, err := normalizeSystemConfig(input)
	if err != nil {
		return err
	}
	existing, err := readGatewayState()
	if err != nil {
		return err
	}
	fresh := !existing.Configured
	oldForward := existing.OldIPForward
	if existing.Configured {
		if !sameSystemConfig(existing.Config, cfg) {
			return fmt.Errorf("Gateway Linux networking is already configured for %s %s/%d (WAN %s); disable it before changing interfaces or network",
				existing.Config.LANInterface, existing.Config.LANIP, existing.Config.LANPrefix, valueOrNone(existing.Config.WANInterface))
		}
		if existing.LANNetwork != "" {
			lanNetwork = existing.LANNetwork
		}
	} else {
		out, err := command("sysctl", "-n", "net.ipv4.ip_forward")
		if err != nil {
			return fmt.Errorf("read net.ipv4.ip_forward: %w", err)
		}
		oldForward = strings.TrimSpace(string(out))
		if oldForward != "0" && oldForward != "1" {
			return fmt.Errorf("unexpected net.ipv4.ip_forward value %q", oldForward)
		}
		if err := writeGatewayState(cfg, lanNetwork, oldForward); err != nil {
			return err
		}
	}

	fail := func(step string, stepErr error) error {
		if fresh {
			rollbackFreshSystem(oldForward)
		}
		return fmt.Errorf("%s: %w", step, stepErr)
	}
	if _, err := command("sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return fail("enable IPv4 forwarding", err)
	}
	if err := enablePolicyRouting(); err != nil {
		return fail("configure policy routing", err)
	}
	if err := applyGatewayNFT(lanNetwork, cfg.WANInterface); err != nil {
		return fail("configure nftables", err)
	}
	if err := persistGatewayFirewall(); err != nil {
		return fail("persist nftables", err)
	}
	return nil
}

func valueOrNone(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

// DisableSystem removes Gateway-owned Linux rules/services. If a compatible
// state file exists, the previous IPv4 forwarding value is restored.
func DisableSystem() error {
	state, stateErr := readGatewayState()
	disableFirewallPersistence()
	deleteGatewayNFT()
	disablePolicyRouting()

	var failures []string
	if stateErr != nil {
		failures = append(failures, stateErr.Error())
	} else if state.Configured {
		if err := restoreIPForward(state.OldIPForward); err != nil {
			failures = append(failures, fmt.Sprintf("restore IPv4 forwarding: %v", err))
		}
	}
	if len(failures) == 0 {
		if err := os.Remove(gatewayStatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			failures = append(failures, fmt.Sprintf("remove Gateway system state: %v", err))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%s", strings.Join(failures, "; "))
	}
	return nil
}

func outputContains(name string, args []string, needle string) bool {
	out, err := command(name, args...)
	return err == nil && strings.Contains(string(out), needle)
}

// GetSystemState verifies live Linux state instead of trusting gateway.env.
// This lets the web UI detect and repair a reboot/manual-rule-loss scenario.
func GetSystemState() (SystemState, error) {
	state, err := readGatewayState()
	if err != nil {
		return state, err
	}
	if !state.Configured {
		return state, nil
	}
	forward, forwardErr := command("sysctl", "-n", "net.ipv4.ip_forward")
	if forwardErr != nil {
		return state, fmt.Errorf("check IPv4 forwarding: %w", forwardErr)
	}
	state.IPForward = strings.TrimSpace(string(forward)) == "1"

	rules, ruleErr := command("ip", "rule", "show")
	routes, routeErr := command("ip", "route", "show", "table", "100")
	if ruleErr != nil || routeErr != nil {
		if ruleErr != nil {
			return state, fmt.Errorf("check Gateway policy rule: %w", ruleErr)
		}
		return state, fmt.Errorf("check Gateway policy route: %w", routeErr)
	}
	ruleText := string(rules)
	routeText := string(routes)
	state.PolicyRoute = (strings.Contains(ruleText, "fwmark 0x40/0xc0") || strings.Contains(ruleText, "fwmark 0x40/0x000000c0")) && strings.Contains(routeText, "local default dev lo")
	state.Firewall = outputContains("nft", []string{"list", "table", "inet", "xui_gateway"}, "table inet xui_gateway")
	state.NAT = state.Config.WANInterface == "" || outputContains("nft", []string{"list", "table", "ip", "xui_gateway_nat"}, "table ip xui_gateway_nat")
	state.Active = state.IPForward && state.PolicyRoute && state.Firewall && state.NAT
	return state, nil
}
