package service

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	gatewayStatePath          = "/etc/x-ui/gateway.env"
	gatewayNFTPath            = "/etc/x-ui/gateway.nft"
	gatewayRoutingServicePath = "/etc/systemd/system/xui-gateway-routing.service"
	gatewayFirewallServicePath = "/etc/systemd/system/xui-gateway-firewall.service"
	gatewayRestoreScriptPath  = "/usr/local/sbin/xui-gateway-firewall-restore"
)

type GatewayNetworkConfig struct {
	LANInterface string `json:"lanInterface"`
	LANIP        string `json:"lanIP"`
	LANPrefix    int    `json:"lanPrefix"`
	WANInterface string `json:"wanInterface,omitempty"`
}

type GatewayNetworkStatus struct {
	Configured   bool                 `json:"configured"`
	Config       GatewayNetworkConfig `json:"config"`
	Forwarding   bool                 `json:"forwarding"`
	PolicyRoute  bool                 `json:"policyRoute"`
	NFTables     bool                 `json:"nftables"`
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
	if out, err := runGatewayCommand(ctx, "sysctl", "-n", "net.ipv4.ip_forward"); err == nil {
		status.Forwarding = strings.TrimSpace(out) == "1"
	}
	if out, err := runGatewayCommand(ctx, "ip", "rule", "show"); err == nil {
		status.PolicyRoute = strings.Contains(out, "fwmark 0x40/0xc0 lookup 100") || strings.Contains(out, "fwmark 0x40/0xc0 table 100")
	}
	if _, err := runGatewayCommand(ctx, "nft", "list", "table", "inet", "xui_gateway"); err == nil {
		status.NFTables = true
	}
	return status
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

	_ = runGatewayCommandOnly(ctx, "systemctl", "disable", "--now", "xui-gateway-firewall.service")
	_ = runGatewayCommandOnly(ctx, "systemctl", "disable", "--now", "xui-gateway-routing.service")
	_ = os.Remove(gatewayFirewallServicePath)
	_ = os.Remove(gatewayRoutingServicePath)
	_ = os.Remove(gatewayRestoreScriptPath)
	_ = os.Remove(gatewayNFTPath)
	_ = runGatewayCommandOnly(ctx, "nft", "delete", "table", "inet", "xui_gateway")
	_ = runGatewayCommandOnly(ctx, "nft", "delete", "table", "ip", "xui_gateway_nat")
	_ = runGatewayCommandOnly(ctx, "ip", "rule", "del", "fwmark", "0x40/0xc0", "table", "100")
	_ = runGatewayCommandOnly(ctx, "ip", "route", "del", "local", "default", "dev", "lo", "table", "100")

	if stateErr == nil && (state.OldForwarding == "0" || state.OldForwarding == "1") {
		if _, err := runGatewayCommand(ctx, "sysctl", "-w", "net.ipv4.ip_forward="+state.OldForwarding); err != nil {
			return fmt.Errorf("restore net.ipv4.ip_forward: %w", err)
		}
	}
	_ = os.Remove(gatewayStatePath)
	_ = runGatewayCommandOnly(ctx, "systemctl", "daemon-reload")
	return nil
}

func applyGatewayNetwork(ctx context.Context, cfg GatewayNetworkConfig, lanNetwork, oldForwarding string, persistState bool) (err error) {
	if err = os.MkdirAll(filepath.Dir(gatewayStatePath), 0o755); err != nil {
		return fmt.Errorf("create gateway state directory: %w", err)
	}

	rollback := true
	defer func() {
		if !rollback {
			return
		}
		_ = runGatewayCommandOnly(context.Background(), "nft", "delete", "table", "inet", "xui_gateway")
		_ = runGatewayCommandOnly(context.Background(), "nft", "delete", "table", "ip", "xui_gateway_nat")
		_ = runGatewayCommandOnly(context.Background(), "ip", "rule", "del", "fwmark", "0x40/0xc0", "table", "100")
		_ = runGatewayCommandOnly(context.Background(), "ip", "route", "del", "local", "default", "dev", "lo", "table", "100")
		if persistState {
			_, _ = runGatewayCommand(context.Background(), "sysctl", "-w", "net.ipv4.ip_forward="+oldForwarding)
		}
	}()

	if _, err = runGatewayCommand(ctx, "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
		return fmt.Errorf("enable IPv4 forwarding: %w", err)
	}

	_ = runGatewayCommandOnly(ctx, "ip", "rule", "del", "fwmark", "0x40/0xc0", "table", "100")
	if _, err = runGatewayCommand(ctx, "ip", "rule", "add", "fwmark", "0x40/0xc0", "table", "100"); err != nil {
		return fmt.Errorf("add Gateway policy rule: %w", err)
	}
	if _, err = runGatewayCommand(ctx, "ip", "route", "replace", "local", "default", "dev", "lo", "table", "100"); err != nil {
		return fmt.Errorf("add Gateway policy route: %w", err)
	}

	_ = runGatewayCommandOnly(ctx, "nft", "delete", "table", "inet", "xui_gateway")
	_ = runGatewayCommandOnly(ctx, "nft", "delete", "table", "ip", "xui_gateway_nat")
	nftRules := gatewayNFTRules(lanNetwork, cfg.WANInterface)
	if err = runGatewayCommandInput(ctx, nftRules, "nft", "-f", "-"); err != nil {
		return fmt.Errorf("apply Gateway nftables rules: %w", err)
	}
	if err = os.WriteFile(gatewayNFTPath, []byte(nftRules), 0o600); err != nil {
		return fmt.Errorf("save Gateway nftables rules: %w", err)
	}
	if err = installGatewayNetworkUnits(ctx); err != nil {
		return err
	}
	if persistState {
		if err = saveGatewayNetworkState(gatewayNetworkState{OldForwarding: oldForwarding, Config: cfg, LANNetwork: lanNetwork}); err != nil {
			return err
		}
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
	return state, nil
}

func installGatewayNetworkUnits(ctx context.Context) error {
	routingUnit := `[Unit]
Description=3X-UI Gateway Mode policy routing
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
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

func gatewayNFTRules(lanNetwork, wanInterface string) string {
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
}%s`, lanNetwork, nat)
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
