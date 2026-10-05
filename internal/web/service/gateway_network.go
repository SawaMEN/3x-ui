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
	"strconv"
	"strings"
)

const (
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
	Active      bool                 `json:"active"`
	Persistent  bool                 `json:"persistent"`
	Config      GatewayNetworkConfig `json:"config"`
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
	status := GatewayNetworkStatus{}

	state, stateErr := loadGatewayNetworkState()
	if stateErr == nil {
		status.Configured = true
		status.Config = state.Config
	} else if _, err := os.Stat(gatewayStatePath); err == nil {
		// Keep a corrupt/stale state visible so the UI can offer cleanup instead
		// of silently pretending that Gateway networking is absent.
		status.Active = true
	}

	if out, err := runGatewayCommand(ctx, "sysctl", "-n", "net.ipv4.ip_forward"); err == nil {
		status.Forwarding = strings.TrimSpace(out) == "1"
	}

	rulePresent := false
	if out, err := runGatewayCommand(ctx, "ip", "rule", "show"); err == nil {
		rulePresent = gatewayPolicyRulePresent(out)
	}
	routePresent := false
	if out, err := runGatewayCommand(ctx, "ip", "route", "show", "table", "100"); err == nil {
		routePresent = gatewayPolicyRoutePresent(out)
	}
	status.PolicyRoute = rulePresent && routePresent

	mainNFT := false
	if _, err := runGatewayCommand(ctx, "nft", "list", "table", "inet", "xui_gateway"); err == nil {
		mainNFT = true
	}
	natNFT := false
	if _, err := runGatewayCommand(ctx, "nft", "list", "table", "ip", "xui_gateway_nat"); err == nil {
		natNFT = true
	}
	status.NFTables = mainNFT && (!status.Configured || state.Config.WANInterface == "" || natNFT)
	status.Active = status.Active || rulePresent || routePresent || mainNFT || natNFT

	_, routingUnitErr := os.Stat(gatewayRoutingServicePath)
	_, firewallUnitErr := os.Stat(gatewayFirewallServicePath)
	_, restoreScriptErr := os.Stat(gatewayRestoreScriptPath)
	_, nftFileErr := os.Stat(gatewayNFTPath)
	routingUnitPresent := routingUnitErr == nil
	firewallUnitPresent := firewallUnitErr == nil
	restoreScriptPresent := restoreScriptErr == nil
	nftFilePresent := nftFileErr == nil
	status.Persistent = routingUnitPresent && firewallUnitPresent && restoreScriptPresent && nftFilePresent
	// A partially installed/removed persistence set is still owned Gateway
	// state. Surface it as active so the UI offers cleanup instead of hiding it.
	status.Active = status.Active || routingUnitPresent || firewallUnitPresent || restoreScriptPresent || nftFilePresent

	return status
}

func gatewayPolicyRulePresent(out string) bool {
	return strings.Contains(out, "fwmark 0x40/0xc0 lookup 100") ||
		strings.Contains(out, "fwmark 0x40/0xc0 table 100")
}

func gatewayPolicyRoutePresent(out string) bool {
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == "local" && fields[1] == "default" && fields[2] == "dev" && fields[3] == "lo" {
			return true
		}
	}
	return false
}

func (s *GatewayNetworkService) Enable(ctx context.Context, cfg GatewayNetworkConfig) error {
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
	}
	if _, statErr := os.Stat(gatewayStatePath); statErr == nil {
		return fmt.Errorf("Gateway network state is invalid; disable Gateway Mode to clean it up before enabling again")
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("check Gateway network state: %w", statErr)
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
	state, stateErr := loadGatewayNetworkState()
	statePresent := false
	if _, err := os.Stat(gatewayStatePath); err == nil {
		statePresent = true
	}

	cleanupErr := cleanupGatewayNetworkRuntime(ctx)
	persistenceErr := removeGatewayNetworkPersistence(ctx, false)

	var forwardingErr error
	if stateErr == nil && (state.OldForwarding == "0" || state.OldForwarding == "1") {
		if _, err := runGatewayCommand(ctx, "sysctl", "-w", "net.ipv4.ip_forward="+state.OldForwarding); err != nil {
			forwardingErr = fmt.Errorf("restore net.ipv4.ip_forward: %w", err)
		}
	} else if statePresent && stateErr != nil {
		forwardingErr = fmt.Errorf("read Gateway state for forwarding restore: %w", stateErr)
	}

	// If restoring the original forwarding value failed, preserve the state so
	// the operator can retry. A corrupt state cannot be retried meaningfully and
	// is removed after reporting the problem.
	if forwardingErr == nil || stateErr != nil {
		if err := os.Remove(gatewayStatePath); err != nil && !os.IsNotExist(err) {
			persistenceErr = errors.Join(persistenceErr, fmt.Errorf("remove Gateway state: %w", err))
		}
	}
	if err := runGatewayCommandOnly(ctx, "systemctl", "daemon-reload"); err != nil {
		persistenceErr = errors.Join(persistenceErr, fmt.Errorf("reload systemd: %w", err))
	}

	return errors.Join(cleanupErr, persistenceErr, forwardingErr)
}

func applyGatewayNetwork(ctx context.Context, cfg GatewayNetworkConfig, lanNetwork, oldForwarding string, persistState bool) (err error) {
	if err = os.MkdirAll(filepath.Dir(gatewayStatePath), 0o755); err != nil {
		return fmt.Errorf("create gateway state directory: %w", err)
	}

	rollback := true
	defer func() {
		if !rollback || !persistState {
			return
		}
		_ = cleanupGatewayNetworkRuntime(context.Background())
		_ = removeGatewayNetworkPersistence(context.Background(), true)
		_, _ = runGatewayCommand(context.Background(), "sysctl", "-w", "net.ipv4.ip_forward="+oldForwarding)
	}()

	if _, err = runGatewayCommand(ctx, "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return fmt.Errorf("enable IPv4 forwarding: %w", err)
	}

	_ = deleteGatewayPolicyRule(ctx)
	if _, err = runGatewayCommand(ctx, "ip", "rule", "add", "fwmark", "0x40/0xc0", "table", "100"); err != nil {
		return fmt.Errorf("add Gateway policy rule: %w", err)
	}
	if _, err = runGatewayCommand(ctx, "ip", "route", "replace", "local", "default", "dev", "lo", "table", "100"); err != nil {
		return fmt.Errorf("add Gateway policy route: %w", err)
	}

	_ = deleteGatewayNFTTable(ctx, "inet", "xui_gateway")
	_ = deleteGatewayNFTTable(ctx, "ip", "xui_gateway_nat")
	nftRules := gatewayNFTRules(cfg.LANInterface, lanNetwork, cfg.WANInterface)
	if err = runGatewayCommandInput(ctx, nftRules, "nft", "-f", "-"); err != nil {
		return fmt.Errorf("apply Gateway nftables rules: %w", err)
	}
	if err = os.WriteFile(gatewayNFTPath, []byte(nftRules), 0o600); err != nil {
		return fmt.Errorf("save Gateway nftables rules: %w", err)
	}
	if persistState {
		if err = saveGatewayNetworkState(gatewayNetworkState{OldForwarding: oldForwarding, Config: cfg, LANNetwork: lanNetwork}); err != nil {
			return err
		}
	}
	if err = installGatewayNetworkUnits(ctx); err != nil {
		return err
	}
	rollback = false
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
	if cfg.LANInterface == "" {
		return cfg, "", fmt.Errorf("LAN interface is required")
	}
	if !safeGatewayInterfaceName(cfg.LANInterface) {
		return cfg, "", fmt.Errorf("LAN interface %q contains unsupported characters", cfg.LANInterface)
	}
	lanInterface, err := net.InterfaceByName(cfg.LANInterface)
	if err != nil {
		return cfg, "", fmt.Errorf("LAN interface %q does not exist", cfg.LANInterface)
	}
	ip := net.ParseIP(cfg.LANIP)
	if ip == nil || ip.To4() == nil {
		return cfg, "", fmt.Errorf("LAN IP must be a valid IPv4 address")
	}
	ip = ip.To4()
	assigned, err := gatewayInterfaceHasIPv4(lanInterface, ip)
	if err != nil {
		return cfg, "", fmt.Errorf("read addresses for LAN interface %q: %w", cfg.LANInterface, err)
	}
	if !assigned {
		return cfg, "", fmt.Errorf("LAN IP %s is not assigned to interface %s", ip.String(), cfg.LANInterface)
	}
	_, network, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip.String(), cfg.LANPrefix))
	if err != nil {
		return cfg, "", fmt.Errorf("calculate LAN network: %w", err)
	}
	if cfg.WANInterface != "" {
		if !safeGatewayInterfaceName(cfg.WANInterface) {
			return cfg, "", fmt.Errorf("WAN interface %q contains unsupported characters", cfg.WANInterface)
		}
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

func safeGatewayInterfaceName(name string) bool {
	if name == "" || len(name) > 15 {
		return false
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '_', '-', '.', ':':
			continue
		default:
			return false
		}
	}
	return true
}

func gatewayInterfaceHasIPv4(iface *net.Interface, target net.IP) (bool, error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return false, err
	}
	for _, addr := range addrs {
		var address string
		switch value := addr.(type) {
		case *net.IPNet:
			address = value.IP.String()
		case *net.IPAddr:
			address = value.IP.String()
		default:
			address = strings.SplitN(addr.String(), "/", 2)[0]
		}
		ip := net.ParseIP(address)
		if ip != nil && ip.To4() != nil && ip.To4().Equal(target) {
			return true, nil
		}
	}
	return false, nil
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
	return state, nil
}

func installGatewayNetworkUnits(ctx context.Context) error {
	routingUnit := `[Unit]
Description=3X-UI Gateway Mode policy routing
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStartPre=-/usr/sbin/ip rule del fwmark 0x40/0xc0 table 100
ExecStart=/usr/sbin/ip rule add fwmark 0x40/0xc0 table 100
ExecStart=/usr/sbin/ip route replace local default dev lo table 100
ExecStop=-/usr/sbin/ip rule del fwmark 0x40/0xc0 table 100
ExecStop=-/usr/sbin/ip route del local default dev lo table 100
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`
	restoreScript := `#!/usr/bin/env bash
set -euo pipefail
STATE="/etc/x-ui/gateway.env"
RULES="/etc/x-ui/gateway.nft"
[[ -f "${STATE}" && -s "${RULES}" ]] || exit 0
/usr/sbin/sysctl -w net.ipv4.ip_forward=1
/usr/sbin/nft delete table inet xui_gateway 2>/dev/null || true
/usr/sbin/nft delete table ip xui_gateway_nat 2>/dev/null || true
/usr/sbin/nft -f "${RULES}"
`
	firewallUnit := `[Unit]
Description=3X-UI Gateway Mode Firewall Restore
Wants=network-online.target xui-gateway-routing.service
After=network-online.target nftables.service xui-gateway-routing.service

[Service]
Type=oneshot
ExecStart=/usr/local/sbin/xui-gateway-firewall-restore
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`
	if err := atomicWriteGatewayFile(gatewayRoutingServicePath, []byte(routingUnit), 0o644); err != nil {
		return fmt.Errorf("write Gateway routing service: %w", err)
	}
	if err := atomicWriteGatewayFile(gatewayRestoreScriptPath, []byte(restoreScript), 0o700); err != nil {
		return fmt.Errorf("write Gateway restore script: %w", err)
	}
	if err := atomicWriteGatewayFile(gatewayFirewallServicePath, []byte(firewallUnit), 0o644); err != nil {
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

func atomicWriteGatewayFile(path string, content []byte, mode os.FileMode) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, mode); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func removeGatewayNetworkPersistence(ctx context.Context, removeState bool) error {
	var errs []error
	for _, unit := range []struct {
		name string
		path string
	}{
		{name: "xui-gateway-firewall.service", path: gatewayFirewallServicePath},
		{name: "xui-gateway-routing.service", path: gatewayRoutingServicePath},
	} {
		if _, err := os.Stat(unit.path); err == nil {
			if cmdErr := runGatewayCommandOnly(ctx, "systemctl", "disable", "--now", unit.name); cmdErr != nil {
				errs = append(errs, fmt.Errorf("disable %s: %w", unit.name, cmdErr))
			}
		} else if !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("check %s: %w", unit.path, err))
		}
	}
	for _, path := range []string{gatewayFirewallServicePath, gatewayRoutingServicePath, gatewayRestoreScriptPath, gatewayNFTPath} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove %s: %w", path, err))
		}
	}
	if removeState {
		if err := os.Remove(gatewayStatePath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("remove %s: %w", gatewayStatePath, err))
		}
	}
	if err := runGatewayCommandOnly(ctx, "systemctl", "daemon-reload"); err != nil {
		errs = append(errs, fmt.Errorf("reload systemd: %w", err))
	}
	return errors.Join(errs...)
}

func cleanupGatewayNetworkRuntime(ctx context.Context) error {
	var errs []error
	if err := deleteGatewayNFTTable(ctx, "inet", "xui_gateway"); err != nil {
		errs = append(errs, err)
	}
	if err := deleteGatewayNFTTable(ctx, "ip", "xui_gateway_nat"); err != nil {
		errs = append(errs, err)
	}
	if err := deleteGatewayPolicyRule(ctx); err != nil {
		errs = append(errs, err)
	}
	if out, err := runGatewayCommand(ctx, "ip", "route", "show", "table", "100"); err != nil {
		errs = append(errs, fmt.Errorf("inspect Gateway policy route: %w", err))
	} else if gatewayPolicyRoutePresent(out) {
		if _, err := runGatewayCommand(ctx, "ip", "route", "del", "local", "default", "dev", "lo", "table", "100"); err != nil {
			errs = append(errs, fmt.Errorf("delete Gateway policy route: %w", err))
		}
	}
	return errors.Join(errs...)
}

func deleteGatewayPolicyRule(ctx context.Context) error {
	out, err := runGatewayCommand(ctx, "ip", "rule", "show")
	if err != nil {
		return fmt.Errorf("inspect Gateway policy rule: %w", err)
	}
	for gatewayPolicyRulePresent(out) {
		if _, err := runGatewayCommand(ctx, "ip", "rule", "del", "fwmark", "0x40/0xc0", "table", "100"); err != nil {
			return fmt.Errorf("delete Gateway policy rule: %w", err)
		}
		out, err = runGatewayCommand(ctx, "ip", "rule", "show")
		if err != nil {
			return fmt.Errorf("recheck Gateway policy rule: %w", err)
		}
	}
	return nil
}

func deleteGatewayNFTTable(ctx context.Context, family, name string) error {
	_, err := runGatewayCommand(ctx, "nft", "list", "table", family, name)
	if err != nil {
		if gatewayNFTObjectMissing(err) {
			return nil
		}
		return fmt.Errorf("inspect nftables table %s %s: %w", family, name, err)
	}
	if _, err := runGatewayCommand(ctx, "nft", "delete", "table", family, name); err != nil {
		return fmt.Errorf("delete nftables table %s %s: %w", family, name, err)
	}
	return nil
}

func gatewayNFTObjectMissing(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such file or directory") || strings.Contains(message, "does not exist")
}

func gatewayNFTRules(lanInterface, lanNetwork, wanInterface string) string {
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
    set interface {
        type ipv4_addr
        flags interval
        auto-merge
        elements = { 127.0.0.0/8, %s }
    }
    chain tp_pre {
        meta l4proto { tcp, udp } fib saddr type != local fib daddr type != local jump tp_rule
        meta l4proto { tcp, udp } meta mark & 0x000000c0 == 0x00000040 tproxy ip to 127.0.0.1:52345
    }
    chain prerouting {
        type filter hook prerouting priority mangle - 5;
        policy accept;
        iifname "%s" meta nfproto ipv4 jump tp_pre
    }
    chain tp_rule {
        meta mark set ct mark
        meta mark & 0x000000c0 == 0x00000040 return
        ip daddr @interface return
        ip daddr @whitelist return
        jump tp_mark
    }
    chain tp_mark {
        tcp flags syn / fin,syn,rst,ack meta mark set meta mark | 0x00000040
        meta l4proto udp ct state new meta mark set meta mark | 0x00000040
        ct mark set meta mark
    }
}%s`, lanNetwork, lanInterface, nat)
}

func runGatewayCommand(ctx context.Context, name string, args ...string) (string, error) {
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
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}
