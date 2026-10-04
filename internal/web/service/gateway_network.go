package service

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	gatewayRPFilterPath        = "/etc/x-ui/gateway-rp-filter"
	gatewayStatePath           = "/etc/x-ui/gateway.env"
	gatewayNFTPath             = "/etc/x-ui/gateway.nft"
	gatewayRoutingServicePath  = "/etc/systemd/system/xui-gateway-routing.service"
	gatewayFirewallServicePath = "/etc/systemd/system/xui-gateway-firewall.service"
	gatewayRestoreScriptPath   = "/usr/local/sbin/xui-gateway-firewall-restore"
)

type GatewayNetworkConfig struct {
	LANInterface string `json:"lanInterface"`
	LANIP        string `json:"lanIP"`
	LANPrefix    int    `json:"lanPrefix"`
	WANInterface string `json:"wanInterface,omitempty"`
}

type GatewayNetworkStatus struct {
	Configured  bool                 `json:"configured"`
	Config      GatewayNetworkConfig `json:"config"`
	RPFilter    bool                 `json:"rpFilter"`
	Forwarding  bool                 `json:"forwarding"`
	PolicyRoute bool                 `json:"policyRoute"`
	NFTables    bool                 `json:"nftables"`
}

type GatewayNetworkService struct{}

type gatewayNetworkState struct {
	OldForwarding string
	Config        GatewayNetworkConfig
	LANNetwork    string
}

func (s *GatewayNetworkService) Status(ctx context.Context) GatewayNetworkStatus {
	state, err := loadGatewayNetworkState()
	if err != nil {
		return GatewayNetworkStatus{}
	}
	status := GatewayNetworkStatus{Configured: true, Config: state.Config}
	status.RPFilter = true
	for _, name := range []string{"all", state.Config.LANInterface} {
		if value, err := runGatewayCommand(ctx, "sysctl", "-n", "net/ipv4/conf/"+name+"/rp_filter"); err != nil || strings.TrimSpace(value) != "0" {
			status.RPFilter = false
		}
	}
	if out, err := runGatewayCommand(ctx, "sysctl", "-n", "net.ipv4.ip_forward"); err == nil {
		status.Forwarding = strings.TrimSpace(out) == "1"
	}
	if out, err := runGatewayCommand(ctx, "ip", "rule", "show"); err == nil {
		status.PolicyRoute = gatewayPolicyRulePresent(out)
		if routes, routeErr := runGatewayCommand(ctx, "ip", "-4", "route", "show", "table", "100"); routeErr != nil || !strings.Contains(routes, "local default dev lo") {
			status.PolicyRoute = false
		}
	}
	if _, err := runGatewayCommand(ctx, "nft", "list", "table", "inet", "xui_gateway"); err == nil {
		status.NFTables = true
	}
	return status
}

func (s *GatewayNetworkService) Enable(ctx context.Context, cfg GatewayNetworkConfig) error {
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return fmt.Errorf("Gateway network setup requires Linux and root permissions")
	}
	for _, name := range []string{"ip", "nft", "sysctl", "systemctl"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("Gateway requires %s: %w", name, err)
		}
	}
	normalized, lanNetwork, err := normalizeGatewayNetworkConfig(cfg)
	if err != nil {
		return err
	}

	if state, stateErr := loadGatewayNetworkState(); stateErr == nil {
		if state.Config != normalized {
			return fmt.Errorf("Gateway network is already configured for %s (%s); disable it before changing network settings", state.Config.LANInterface, state.LANNetwork)
		}
		// Reconcile a partially lost runtime state without overwriting the
		// forwarding value captured before Gateway Mode was first enabled.
		return applyGatewayNetwork(ctx, normalized, lanNetwork, state.OldForwarding, false)
	} else if !os.IsNotExist(stateErr) {
		return fmt.Errorf("read Gateway recovery state: %w", stateErr)
	}
	// Never replace another VPN's routes in table 100.
	rules, err := runGatewayCommand(ctx, "ip", "-4", "rule", "show")
	if err != nil {
		return err
	}
	if gatewayTableInUse(rules) {
		return fmt.Errorf("policy routing table 100 is already in use")
	}
	routes, routeErr := runGatewayCommand(ctx, "ip", "-4", "route", "show", "table", "100")
	if routeErr != nil && !strings.Contains(routes, "FIB table does not exist") {
		return routeErr
	}
	if routeErr == nil && strings.TrimSpace(routes) != "" {
		return fmt.Errorf("policy routing table 100 is already in use")
	}

	oldForwarding, err := runGatewayCommand(ctx, "sysctl", "-n", "net.ipv4.ip_forward")
	if err != nil {
		return fmt.Errorf("read net.ipv4.ip_forward: %w", err)
	}
	oldForwarding = strings.TrimSpace(oldForwarding)
	if oldForwarding != "0" && oldForwarding != "1" {
		return fmt.Errorf("unexpected net.ipv4.ip_forward value %q", oldForwarding)
	}
	return applyGatewayNetwork(ctx, normalized, lanNetwork, oldForwarding, true)
}

func (s *GatewayNetworkService) Disable(ctx context.Context) error {
	state, err := loadGatewayNetworkState()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read Gateway recovery state: %w", err)
	}
	// Remove interception first. Keep recovery files whenever cleanup fails.
	tables, err := runGatewayCommand(ctx, "nft", "list", "tables")
	if err != nil {
		return err
	}
	for _, table := range []struct{ family, name string }{{"inet", "xui_gateway"}, {"ip", "xui_gateway_nat"}} {
		if strings.Contains(tables, "table "+table.family+" "+table.name+"\n") {
			if err := runGatewayCommandOnly(ctx, "nft", "delete", "table", table.family, table.name); err != nil {
				return err
			}
		}
	}
	for _, unit := range []struct{ path, name string }{{gatewayFirewallServicePath, "xui-gateway-firewall.service"}, {gatewayRoutingServicePath, "xui-gateway-routing.service"}} {
		if _, statErr := os.Stat(unit.path); statErr == nil {
			if err := runGatewayCommandOnly(ctx, "systemctl", "disable", "--now", unit.name); err != nil {
				return err
			}
		}
	}
	rules, err := runGatewayCommand(ctx, "ip", "-4", "rule", "show")
	if err != nil {
		return err
	}
	if gatewayPolicyRulePresent(rules) {
		if err := runGatewayCommandOnly(ctx, "ip", "-4", "rule", "del", "fwmark", "0x40/0xc0", "table", "100"); err != nil {
			return err
		}
	}
	routes, err := runGatewayCommand(ctx, "ip", "-4", "route", "show", "table", "100")
	if err != nil && !strings.Contains(routes, "FIB table does not exist") {
		return err
	}
	if strings.Contains(routes, "local default dev lo") {
		if err := runGatewayCommandOnly(ctx, "ip", "-4", "route", "del", "local", "default", "dev", "lo", "table", "100"); err != nil {
			return err
		}
	}
	if _, err := runGatewayCommand(ctx, "sysctl", "-w", "net.ipv4.ip_forward="+state.OldForwarding); err != nil {
		return fmt.Errorf("restore IPv4 forwarding: %w", err)
	}
	if err := restoreGatewayRPFilter(ctx); err != nil {
		return err
	}
	for _, path := range []string{gatewayRPFilterPath, gatewayFirewallServicePath, gatewayRoutingServicePath, gatewayRestoreScriptPath, gatewayNFTPath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := runGatewayCommandOnly(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	return os.Remove(gatewayStatePath)
}

func applyGatewayNetwork(ctx context.Context, cfg GatewayNetworkConfig, lanNetwork, oldForwarding string, persistState bool) (err error) {
	if err = os.MkdirAll(filepath.Dir(gatewayStatePath), 0o755); err != nil {
		return fmt.Errorf("create gateway state directory: %w", err)
	}

	// Validate a replacement transaction before changing the live firewall.
	batch := gatewayNFTRules(lanNetwork, cfg.WANInterface, cfg.LANInterface)
	tables, listErr := runGatewayCommand(ctx, "nft", "list", "tables")
	if listErr != nil {
		return listErr
	}
	for _, table := range []struct{ family, name string }{{"inet", "xui_gateway"}, {"ip", "xui_gateway_nat"}} {
		if strings.Contains(tables, "table "+table.family+" "+table.name+"\n") {
			if persistState {
				return fmt.Errorf("Gateway firewall exists without recovery state; clean it up first")
			}
			batch = "delete table " + table.family + " " + table.name + "\n" + batch
		}
	}
	if err = runGatewayCommandInput(ctx, batch, "nft", "-c", "-f", "-"); err != nil {
		return fmt.Errorf("validate Gateway nftables: %w", err)
	}
	if persistState {
		if err = saveGatewayNetworkState(gatewayNetworkState{OldForwarding: oldForwarding, Config: cfg, LANNetwork: lanNetwork}); err != nil {
			return err
		}
	}
	defer func() {
		if err != nil && persistState {
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if cleanupErr := (&GatewayNetworkService{}).Disable(cleanupCtx); cleanupErr != nil {
				err = errors.Join(err, fmt.Errorf("Gateway rollback failed: %w", cleanupErr))
			}
		}
	}()

	if _, err = runGatewayCommand(ctx, "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return fmt.Errorf("enable IPv4 forwarding: %w", err)
	}

	if err = configureGatewayRPFilter(ctx, cfg.LANInterface); err != nil {
		return err
	}
	rules, ruleErr := runGatewayCommand(ctx, "ip", "-4", "rule", "show")
	if ruleErr != nil {
		return ruleErr
	}
	if !gatewayPolicyRulePresent(rules) {
		if _, err = runGatewayCommand(ctx, "ip", "-4", "rule", "add", "fwmark", "0x40/0xc0", "table", "100"); err != nil {
			return fmt.Errorf("add Gateway policy rule: %w", err)
		}
	}
	if _, err = runGatewayCommand(ctx, "ip", "-4", "route", "replace", "local", "default", "dev", "lo", "table", "100"); err != nil {
		return fmt.Errorf("add Gateway policy route: %w", err)
	}

	nftRules := gatewayNFTRules(lanNetwork, cfg.WANInterface, cfg.LANInterface)
	if err = runGatewayCommandInput(ctx, batch, "nft", "-f", "-"); err != nil {
		return fmt.Errorf("apply Gateway nftables rules: %w", err)
	}
	if err = os.WriteFile(gatewayNFTPath, []byte(nftRules), 0o600); err != nil {
		return fmt.Errorf("save Gateway nftables rules: %w", err)
	}
	if err = installGatewayNetworkUnits(ctx); err != nil {
		return err
	}
	return nil
}

func normalizeGatewayNetworkConfig(cfg GatewayNetworkConfig) (GatewayNetworkConfig, string, error) {
	cfg.LANInterface = strings.TrimSpace(cfg.LANInterface)
	cfg.LANIP = strings.TrimSpace(cfg.LANIP)
	cfg.WANInterface = strings.TrimSpace(cfg.WANInterface)
	if cfg.LANPrefix == 0 {
		cfg.LANPrefix = 24
	}
	if cfg.LANPrefix < 1 || cfg.LANPrefix > 32 {
		return cfg, "", fmt.Errorf("LAN prefix must be between 1 and 32")
	}
	if !gatewayInterfaceName.MatchString(cfg.LANInterface) || (cfg.WANInterface != "" && !gatewayInterfaceName.MatchString(cfg.WANInterface)) {
		return cfg, "", fmt.Errorf("invalid Gateway interface name")
	}
	if cfg.LANInterface == "" {
		return cfg, "", fmt.Errorf("LAN interface is required")
	}
	if _, err := net.InterfaceByName(cfg.LANInterface); err != nil {
		return cfg, "", fmt.Errorf("LAN interface %q does not exist", cfg.LANInterface)
	}
	ip := net.ParseIP(cfg.LANIP)
	if ip == nil || ip.To4() == nil {
		return cfg, "", fmt.Errorf("LAN IP must be a valid IPv4 address")
	}
	_, network, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip.String(), cfg.LANPrefix))
	if err != nil {
		return cfg, "", fmt.Errorf("calculate LAN network: %w", err)
	}
	if cfg.WANInterface != "" {
		if _, err := net.InterfaceByName(cfg.WANInterface); err != nil {
			return cfg, "", fmt.Errorf("WAN interface %q does not exist", cfg.WANInterface)
		}
		if cfg.WANInterface == cfg.LANInterface {
			return cfg, "", fmt.Errorf("LAN and WAN interfaces must be different")
		}
	}
	cfg.LANIP = ip.String()
	return cfg, network.String(), nil
}

func saveGatewayNetworkState(state gatewayNetworkState) error {
	content := fmt.Sprintf("IP_FORWARD_OLD=%s\nLAN_IF=%s\nLAN_IP=%s\nLAN_PREFIX=%d\nLAN_NETWORK=%s\nWAN_IF=%s\n",
		state.OldForwarding, state.Config.LANInterface, state.Config.LANIP, state.Config.LANPrefix, state.LANNetwork, state.Config.WANInterface)
	tmp := gatewayStatePath + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write Gateway state: %w", err)
	}
	if err := os.Rename(tmp, gatewayStatePath); err != nil {
		return fmt.Errorf("save Gateway state: %w", err)
	}
	return nil
}

func loadGatewayNetworkState() (gatewayNetworkState, error) {
	f, err := os.Open(gatewayStatePath)
	if err != nil {
		return gatewayNetworkState{}, err
	}
	defer f.Close()
	values := map[string]string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if ok {
			values[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	if err := scanner.Err(); err != nil {
		return gatewayNetworkState{}, err
	}
	prefix, err := strconv.Atoi(values["LAN_PREFIX"])
	if err != nil {
		return gatewayNetworkState{}, fmt.Errorf("invalid Gateway LAN_PREFIX")
	}
	state := gatewayNetworkState{
		OldForwarding: values["IP_FORWARD_OLD"],
		LANNetwork:    values["LAN_NETWORK"],
		Config: GatewayNetworkConfig{
			LANInterface: values["LAN_IF"],
			LANIP:        values["LAN_IP"],
			LANPrefix:    prefix,
			WANInterface: values["WAN_IF"],
		},
	}
	if state.OldForwarding != "0" && state.OldForwarding != "1" {
		return gatewayNetworkState{}, fmt.Errorf("invalid Gateway IP_FORWARD_OLD")
	}
	return state, nil
}

func installGatewayNetworkUnits(ctx context.Context) error {
	routingUnit := `[Unit]
Description=3X-UI Gateway Mode policy routing
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/sbin/ip -4 route replace local default dev lo table 100
ExecStart=/bin/sh -c '/usr/sbin/ip -4 rule show | /usr/bin/grep -q "fwmark 0x40/0xc0 lookup 100" || /usr/sbin/ip -4 rule add fwmark 0x40/0xc0 table 100'
ExecStop=-/usr/sbin/ip -4 rule del fwmark 0x40/0xc0 table 100
ExecStop=-/usr/sbin/ip -4 route del local default dev lo table 100
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`
	restoreScript := `#!/usr/bin/env bash
set -euo pipefail
STATE="/etc/x-ui/gateway.env"
RULES="/etc/x-ui/gateway.nft"
[[ -f "${STATE}" && -s "${RULES}" ]] || exit 0
batch=$(mktemp)
trap 'rm -f "$batch"' EXIT
/usr/sbin/nft list table inet xui_gateway >/dev/null 2>&1 && echo 'delete table inet xui_gateway' >> "$batch" || true
/usr/sbin/nft list table ip xui_gateway_nat >/dev/null 2>&1 && echo 'delete table ip xui_gateway_nat' >> "$batch" || true
cat "$RULES" >> "$batch"
/usr/sbin/nft -c -f "$batch"
/usr/sbin/sysctl -w net.ipv4.ip_forward=1
if [[ -f /etc/x-ui/gateway-rp-filter ]]; then
    while read -r key value; do /usr/sbin/sysctl -w "$key=0"; done < /etc/x-ui/gateway-rp-filter
fi
/usr/sbin/nft -f "$batch"
`
	firewallUnit := `[Unit]
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
	if err := os.WriteFile(gatewayRoutingServicePath, []byte(routingUnit), 0o644); err != nil {
		return fmt.Errorf("write Gateway routing service: %w", err)
	}
	if err := os.WriteFile(gatewayRestoreScriptPath, []byte(restoreScript), 0o700); err != nil {
		return fmt.Errorf("write Gateway restore script: %w", err)
	}
	if err := os.WriteFile(gatewayFirewallServicePath, []byte(firewallUnit), 0o644); err != nil {
		return fmt.Errorf("write Gateway firewall service: %w", err)
	}
	if _, err := runGatewayCommand(ctx, "systemctl", "daemon-reload"); err != nil {
		return fmt.Errorf("reload systemd: %w", err)
	}
	if _, err := runGatewayCommand(ctx, "systemctl", "enable", "xui-gateway-routing.service", "xui-gateway-firewall.service"); err != nil {
		return fmt.Errorf("enable Gateway restore services: %w", err)
	}
	return nil
}

func gatewayNFTRules(lanNetwork, wanInterface, lanInterface string) string {
	nat := ""
	if wanInterface != "" {
		nat = fmt.Sprintf(`

table ip xui_gateway_nat {
    chain postrouting {
        type nat hook postrouting priority srcnat;
        policy accept;
        ip saddr %s oifname "%s" masquerade
    }
}
`, lanNetwork, wanInterface)
	}
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
    }
    chain output {
        type route hook output priority mangle - 5;
        policy accept;
    }
    chain prerouting {
        type filter hook prerouting priority mangle - 5;
        policy accept;
        meta nfproto != ipv4 return
        iifname != %q return
        ip saddr != %s return
        jump tp_pre
    }
    chain tp_rule {
        meta mark set ct mark
        meta mark & 0x000000c0 == 0x00000040 return
        ip daddr @interface return
        ip daddr @whitelist return
        ip6 daddr @whitelist6 return
        ip6 daddr @interface6 return
        jump tp_mark
    }
    chain tp_mark {
        tcp flags syn / fin,syn,rst,ack meta mark set (meta mark & 0xffffff3f) | 0x00000040
        meta l4proto udp ct state new meta mark set (meta mark & 0xffffff3f) | 0x00000040
        ct mark set meta mark
    }
}%s`, lanNetwork, lanInterface, lanNetwork, nat)
}

func runGatewayCommand(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return string(out), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), fmt.Errorf("%s", msg))
		}
		return string(out), fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return string(out), nil
}

func runGatewayCommandOnly(ctx context.Context, name string, args ...string) error {
	_, err := runGatewayCommand(ctx, name, args...)
	return err
}

func runGatewayCommandInput(ctx context.Context, input, name string, args ...string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

var gatewayInterfaceName = regexp.MustCompile(`^[a-zA-Z0-9_.:-]+$`)
var gatewayTableRule = regexp.MustCompile(`(?m)\b(?:lookup|table) 100(?:\s|$)`)
var gatewayPolicyRule = regexp.MustCompile(`(?m)\bfwmark 0x40/0xc0 (?:lookup|table) 100(?:\s|$)`)

func gatewayTableInUse(rules string) bool        { return gatewayTableRule.MatchString(rules) }
func gatewayPolicyRulePresent(rules string) bool { return gatewayPolicyRule.MatchString(rules) }

// Strict reverse-path filtering can discard packets routed to the TPROXY
// listener. Save only the all/LAN knobs and restore them on disable.
func configureGatewayRPFilter(ctx context.Context, iface string) error {
	if _, err := os.Stat(gatewayRPFilterPath); os.IsNotExist(err) {
		var snapshot strings.Builder
		for _, name := range []string{"all", iface} {
			key := "net/ipv4/conf/" + name + "/rp_filter"
			value, err := runGatewayCommand(ctx, "sysctl", "-n", key)
			if err != nil {
				return err
			}
			value = strings.TrimSpace(value)
			if value != "0" && value != "1" && value != "2" {
				return fmt.Errorf("invalid rp_filter value %q", value)
			}
			fmt.Fprintf(&snapshot, "%s %s\n", key, value)
		}
		tmp := gatewayRPFilterPath + ".tmp"
		if err := os.WriteFile(tmp, []byte(snapshot.String()), 0600); err != nil {
			return err
		}
		if err := os.Rename(tmp, gatewayRPFilterPath); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	return applyGatewayRPFilter(ctx, false)
}
func restoreGatewayRPFilter(ctx context.Context) error { return applyGatewayRPFilter(ctx, true) }
func applyGatewayRPFilter(ctx context.Context, restore bool) error {
	data, err := os.ReadFile(gatewayRPFilterPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || !strings.HasPrefix(fields[0], "net/ipv4/conf/") || !strings.HasSuffix(fields[0], "/rp_filter") {
			return fmt.Errorf("invalid Gateway rp_filter snapshot")
		}
		if restore {
			if _, err := os.Stat("/proc/sys/" + fields[0]); os.IsNotExist(err) {
				continue
			}
			if fields[1] != "0" && fields[1] != "1" && fields[1] != "2" {
				return fmt.Errorf("invalid Gateway rp_filter snapshot value")
			}
		} else {
			fields[1] = "0"
		}
		if _, err := runGatewayCommand(ctx, "sysctl", "-w", fields[0]+"="+fields[1]); err != nil {
			return err
		}
	}
	return nil
}
