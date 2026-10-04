package service

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var gatewayRPKey = regexp.MustCompile(`^net/ipv4/conf/[a-zA-Z0-9_.:-]+/rp_filter$`)

func gatewayAtomicWrite(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".gateway-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	defer f.Close()
	if err := f.Chmod(mode); err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func gatewayTablePresent(output, family, name string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "table "+family+" "+name {
			return true
		}
	}
	return false
}

func validateGatewayStoredConfig(state gatewayNetworkState) error {
	cfg := state.Config
	if !gatewayInterfaceName.MatchString(cfg.LANInterface) || (cfg.WANInterface != "" && !gatewayInterfaceName.MatchString(cfg.WANInterface)) || cfg.LANInterface == cfg.WANInterface {
		return fmt.Errorf("invalid stored Gateway interfaces")
	}
	ip := net.ParseIP(cfg.LANIP)
	if ip == nil || ip.To4() == nil || !ip.IsGlobalUnicast() || cfg.LANPrefix < 1 || cfg.LANPrefix > 32 {
		return fmt.Errorf("invalid stored Gateway LAN address/prefix")
	}
	_, network, err := net.ParseCIDR(fmt.Sprintf("%s/%d", cfg.LANIP, cfg.LANPrefix))
	if err != nil || network.String() != state.LANNetwork {
		return fmt.Errorf("stored Gateway LAN network disagrees with its address/prefix")
	}
	return nil
}

func (s *GatewayNetworkService) listenerReady(ctx context.Context) bool {
	for _, proto := range []string{"-t", "-u"} {
		out, err := s.command(ctx, "ss", "-H", "-l", "-n", proto, "sport = :52345")
		if err != nil || strings.TrimSpace(out) == "" {
			return false
		}
	}
	return true
}

// A successful process start can precede socket binding. Wait for both sockets
// before installing interception, so early/failed startup cannot blackhole LAN.
func (s *GatewayNetworkService) WaitListener(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	for {
		if s.listenerReady(ctx) {
			return nil
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return fmt.Errorf("Gateway TCP/UDP listener is not ready: %w", ctx.Err())
		case <-timer.C:
		}
	}
}

// Validate request parameters before any template or runtime change.
func (s *GatewayNetworkService) Validate(ctx context.Context, cfg GatewayNetworkConfig) error {
	if err := s.checkSupport(); err != nil {
		return err
	}
	normalized, network, err := normalizeGatewayNetworkConfig(cfg)
	if err != nil {
		return err
	}
	if err := validateGatewayStoredConfig(gatewayNetworkState{Config: normalized, LANNetwork: network}); err != nil {
		return err
	}
	if normalized.WANInterface != "" {
		if _, err := net.InterfaceByName(normalized.WANInterface); err != nil {
			return fmt.Errorf("WAN interface %q does not exist: %w", normalized.WANInterface, err)
		}
	}
	iface, err := net.InterfaceByName(normalized.LANInterface)
	if err != nil {
		return err
	}
	if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
		return fmt.Errorf("LAN interface must be up and must not be loopback")
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return err
	}
	for _, addr := range addrs {
		ip, _, err := net.ParseCIDR(addr.String())
		if err == nil && ip.Equal(net.ParseIP(normalized.LANIP)) {
			return nil
		}
	}
	return fmt.Errorf("LAN IP %s is not assigned to interface %s", normalized.LANIP, normalized.LANInterface)
}

func gatewayOwnedRoutes(output string) bool {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) != 1 {
		return false
	}
	fields := strings.Fields(lines[0])
	return len(fields) >= 4 && fields[0] == "local" && (fields[1] == "default" || fields[1] == "0.0.0.0/0") && fields[2] == "dev" && fields[3] == "lo"
}

var gatewayOwnedPolicyRule = regexp.MustCompile(`(?m)^([0-9]+):\s+from all fwmark 0x40/0xc0 (?:lookup|table) 100(?: proto [a-zA-Z0-9_-]+)?\s*$`)

func gatewayOwnedRulePriorities(output string) []string {
	matches := gatewayOwnedPolicyRule.FindAllStringSubmatch(output, -1)
	priorities := make([]string, 0, len(matches))
	for _, match := range matches {
		priorities = append(priorities, match[1])
	}
	return priorities
}
func gatewayPolicyRuleCount(output string) int { return len(gatewayOwnedRulePriorities(output)) }
func gatewayForeignRule(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if gatewayTableInUse(line) && gatewayPolicyRuleCount(line) != 1 {
			return true
		}
	}
	return false
}

func gatewayHasOwnedRoute(output string) bool {
	for _, line := range strings.Split(output, "\n") {
		if gatewayOwnedRoutes(line) {
			return true
		}
	}
	return false
}
