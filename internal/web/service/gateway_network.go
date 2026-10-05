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
	"sort"
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
	ListenerReady    bool                 `json:"listenerReady"`
	Configured       bool                 `json:"configured"`
	RecoveryRequired bool                 `json:"recoveryRequired"`
	Config           GatewayNetworkConfig `json:"config"`
	RPFilter         bool                 `json:"rpFilter"`
	Forwarding       bool                 `json:"forwarding"`
	PolicyRoute      bool                 `json:"policyRoute"`
	NAT              bool                 `json:"nat"`
	Persistent       bool                 `json:"persistent"`
	Error            string               `json:"error,omitempty"`
	NFTables         bool                 `json:"nftables"`
}

type GatewayNetworkService struct {
	paths      *gatewayNetworkPaths
	interfaces func() ([]net.Interface, error)
	run        func(context.Context, string, ...string) (string, error)
	input      func(context.Context, string, string, ...string) error
}
type gatewayNetworkPaths struct{ state, rpFilter, nft, routingUnit, firewallUnit, restoreScript string }

func (s *GatewayNetworkService) networkPaths() gatewayNetworkPaths {
	if s.paths != nil {
		return *s.paths
	}
	return gatewayNetworkPaths{gatewayStatePath, gatewayRPFilterPath, gatewayNFTPath, gatewayRoutingServicePath, gatewayFirewallServicePath, gatewayRestoreScriptPath}
}
func (s *GatewayNetworkService) command(ctx context.Context, name string, args ...string) (string, error) {
	if s.run != nil {
		return s.run(ctx, name, args...)
	}
	return runGatewayCommand(ctx, name, args...)
}
func (s *GatewayNetworkService) commandOnly(ctx context.Context, name string, args ...string) error {
	_, err := s.command(ctx, name, args...)
	return err
}
func (s *GatewayNetworkService) commandInput(ctx context.Context, input, name string, args ...string) error {
	if s.input != nil {
		return s.input(ctx, input, name, args...)
	}
	return runGatewayCommandInput(ctx, input, name, args...)
}

type gatewayNetworkState struct {
	OldForwarding string
	Config        GatewayNetworkConfig
	LANNetwork    string
	DesiredActive bool
	// Legacy identifies the pre-unified gateway.env format, which did not
	// manage rp_filter and did not have a NETWORK_ENABLED marker.
	Legacy bool
}

func (s *GatewayNetworkService) Status(ctx context.Context) GatewayNetworkStatus {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	state, err := s.loadGatewayNetworkState()
	if err != nil {
		if _, statErr := os.Stat(s.networkPaths().state); statErr == nil {
			return GatewayNetworkStatus{Configured: true, Error: err.Error()}
		}
		if !os.IsNotExist(err) {
			return GatewayNetworkStatus{Error: err.Error()}
		}
		// A deleted gateway.env must not make an active, panel-owned TPROXY
		// firewall look like a clean disabled Gateway. Surface the residue so
		// callers can explicitly clean it instead of restarting a core behind
		// an invisible interception rule.
		if s.checkSupport() == nil {
			orphaned, inspectErr := s.gatewayOrphaned(ctx)
			if inspectErr != nil {
				return GatewayNetworkStatus{Error: fmt.Sprintf("inspect orphaned Gateway networking: %v", inspectErr)}
			}
			if orphaned {
				return GatewayNetworkStatus{RecoveryRequired: true}
			}
		}
		return GatewayNetworkStatus{}
	}
	status := GatewayNetworkStatus{Configured: true, Config: state.Config}
	if !state.Legacy {
		if snapshotErr := s.validateGatewayRPSnapshot(); snapshotErr != nil {
			status.Error = fmt.Sprintf("Gateway rp_filter recovery snapshot: %v", snapshotErr)
		}
	}
	status.RPFilter = true
	for _, name := range []string{"all", state.Config.LANInterface} {
		if value, err := s.command(ctx, "sysctl", "-n", "net/ipv4/conf/"+name+"/rp_filter"); err != nil || strings.TrimSpace(value) != "0" {
			status.RPFilter = false
		}
	}
	if out, err := s.command(ctx, "sysctl", "-n", "net.ipv4.ip_forward"); err == nil {
		status.Forwarding = strings.TrimSpace(out) == "1"
	}
	if out, err := s.command(ctx, "ip", "-N", "-4", "rule", "show"); err == nil {
		status.PolicyRoute = gatewayPolicyRuleCount(out) == 1 && !gatewayForeignRule(out)
		if routes, routeErr := s.command(ctx, "ip", "-4", "route", "show", "table", "100"); routeErr != nil || !gatewayOwnedRoutes(routes) {
			status.PolicyRoute = false
		}
	}
	if rules, err := s.command(ctx, "nft", "list", "table", "inet", "xui_gateway"); err == nil {
		status.NFTables = strings.Contains(rules, "3x-ui gateway v2") && strings.Contains(rules, "127.0.0.1:52345") && strings.Contains(rules, strconv.Quote(state.Config.LANInterface))
	}
	status.NAT = state.Config.WANInterface == ""
	if !status.NAT {
		if rules, err := s.command(ctx, "nft", "list", "table", "ip", "xui_gateway_nat"); err == nil {
			status.NAT = strings.Contains(rules, "masquerade") && strings.Contains(rules, strconv.Quote(state.Config.WANInterface))
		}
	}
	status.Persistent = state.DesiredActive
	for _, unit := range []string{"xui-gateway-routing.service", "xui-gateway-firewall.service"} {
		if _, err := s.command(ctx, "systemctl", "is-enabled", unit); err != nil {
			status.Persistent = false
		}
	}
	for _, path := range []string{s.networkPaths().rpFilter, s.networkPaths().nft, s.networkPaths().routingUnit, s.networkPaths().firewallUnit, s.networkPaths().restoreScript} {
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			status.Persistent = false
		}
	}
	status.ListenerReady = s.listenerReady(ctx)
	return status
}

func (s *GatewayNetworkService) checkSupport() error {
	if s.run != nil {
		return nil
	}
	if runtime.GOOS != "linux" || os.Geteuid() != 0 {
		return fmt.Errorf("Gateway network setup requires Linux and root permissions")
	}
	for _, name := range []string{"ip", "nft", "sysctl", "systemctl", "bash", "ss", "flock"} {
		if _, err := exec.LookPath(name); err != nil {
			return fmt.Errorf("Gateway requires %s: %w", name, err)
		}
	}
	return nil
}

func (s *GatewayNetworkService) Enable(ctx context.Context, cfg GatewayNetworkConfig) error {
	if err := s.checkSupport(); err != nil {
		return err
	}
	if s.run == nil {
		if err := s.Validate(ctx, cfg); err != nil {
			return err
		}
	}
	if err := s.WaitListener(ctx); err != nil {
		return err
	}
	normalized, lanNetwork, err := normalizeGatewayNetworkConfig(cfg)
	if err != nil {
		return err
	}

	if state, stateErr := s.loadGatewayNetworkState(); stateErr == nil {
		if state.Config != normalized {
			return fmt.Errorf("Gateway network is already configured for %s (%s); disable it before changing network settings", state.Config.LANInterface, state.LANNetwork)
		}
		// The unified implementation changes rp_filter and therefore requires
		// its original-value snapshot for every repair. Recreating a missing
		// snapshot from the already-modified live sysctls would destroy the
		// baseline. Pre-unified state is the only safe exception: it never
		// changed rp_filter, so the current values are still the baseline.
		if !state.Legacy {
			if snapshotErr := s.validateGatewayRPSnapshot(); snapshotErr != nil {
				return fmt.Errorf("Gateway rp_filter recovery snapshot is unavailable; suspend Gateway and restore the snapshot before repair: %w", snapshotErr)
			}
		}
		return s.applyGatewayNetwork(ctx, normalized, lanNetwork, state.OldForwarding, false)
	} else if !os.IsNotExist(stateErr) {
		return fmt.Errorf("read Gateway recovery state: %w", stateErr)
	}
	// Never replace another VPN's routes in table 100.
	rules, err := s.command(ctx, "ip", "-N", "-4", "rule", "show")
	if err != nil {
		return err
	}
	if gatewayTableInUse(rules) {
		return fmt.Errorf("policy routing table 100 is already in use")
	}
	routes, routeErr := s.command(ctx, "ip", "-4", "route", "show", "table", "100")
	if routeErr != nil && !strings.Contains(routes, "FIB table does not exist") {
		return routeErr
	}
	if routeErr == nil && strings.TrimSpace(routes) != "" {
		return fmt.Errorf("policy routing table 100 is already in use")
	}

	oldForwarding, err := s.command(ctx, "sysctl", "-n", "net.ipv4.ip_forward")
	if err != nil {
		return fmt.Errorf("read net.ipv4.ip_forward: %w", err)
	}
	oldForwarding = strings.TrimSpace(oldForwarding)
	if oldForwarding != "0" && oldForwarding != "1" {
		return fmt.Errorf("unexpected net.ipv4.ip_forward value %q", oldForwarding)
	}
	return s.applyGatewayNetwork(ctx, normalized, lanNetwork, oldForwarding, true)
}

// Suspend removes interception before a core is reconfigured. Recovery data
// and policy routing remain until Disable completes or Enable resumes.
func (s *GatewayNetworkService) Suspend(ctx context.Context) error {
	state, err := s.loadGatewayNetworkState()
	if os.IsNotExist(err) {
		orphaned, inspectErr := s.gatewayOrphaned(ctx)
		if inspectErr != nil {
			return fmt.Errorf("inspect Gateway networking without recovery state: %w", inspectErr)
		}
		if !orphaned {
			return nil
		}
		return s.cleanupGatewayWithoutState(ctx, false)
	}
	if err != nil {
		return err
	}

	var recoveryErr error
	if state.Legacy {
		// Older Gateway networking never changed rp_filter. Capture those live
		// values before writing the new state marker so future cleanup has a
		// trustworthy baseline.
		if err := s.captureGatewayRPFilter(ctx, state.Config.LANInterface); err != nil {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf("capture legacy Gateway rp_filter baseline: %w", err))
		}
	} else if err := s.validateGatewayRPSnapshot(); err != nil {
		recoveryErr = errors.Join(recoveryErr, fmt.Errorf("Gateway rp_filter recovery snapshot is unavailable: %w", err))
	}

	state.DesiredActive = false
	if err := s.saveGatewayNetworkState(state); err != nil {
		return errors.Join(recoveryErr, err)
	}

	if state.Legacy {
		// Pre-unified restore units do not understand NETWORK_ENABLED=0. Disable
		// their boot links before returning, even if baseline capture failed.
		for _, unit := range []struct {
			path string
			name string
		}{{s.networkPaths().firewallUnit, "xui-gateway-firewall.service"}, {s.networkPaths().routingUnit, "xui-gateway-routing.service"}} {
			if _, statErr := os.Stat(unit.path); statErr == nil {
				if err := s.commandOnly(ctx, "systemctl", "disable", unit.name); err != nil {
					recoveryErr = errors.Join(recoveryErr, fmt.Errorf("disable legacy %s: %w", unit.name, err))
				}
			}
		}
	}

	if err := s.removeGatewayTables(ctx); err != nil {
		return errors.Join(recoveryErr, err)
	}
	return recoveryErr
}
func (s *GatewayNetworkService) gatewayOrphaned(ctx context.Context) (bool, error) {
	paths := s.networkPaths()
	for _, path := range []string{paths.rpFilter, paths.nft, paths.routingUnit, paths.firewallUnit, paths.restoreScript} {
		if _, err := os.Stat(path); err == nil {
			return true, nil
		} else if !os.IsNotExist(err) {
			return false, err
		}
	}
	tables, err := s.command(ctx, "nft", "list", "tables")
	if err != nil {
		return false, err
	}
	if gatewayTablePresent(tables, "inet", "xui_gateway") || gatewayTablePresent(tables, "ip", "xui_gateway_nat") {
		return true, nil
	}
	rules, err := s.command(ctx, "ip", "-N", "-4", "rule", "show")
	if err != nil {
		return false, err
	}
	return len(gatewayOwnedRulePriorities(rules)) > 0, nil
}

// cleanupGatewayWithoutState removes only resources with an unambiguous 3x-ui
// identity. It deliberately does not guess the previous ip_forward value.
func (s *GatewayNetworkService) cleanupGatewayWithoutState(ctx context.Context, removeFiles bool) error {
	paths := s.networkPaths()
	var cleanupErr error
	for _, unit := range []struct{ path, name string }{{paths.firewallUnit, "xui-gateway-firewall.service"}, {paths.routingUnit, "xui-gateway-routing.service"}} {
		if _, err := os.Stat(unit.path); err == nil {
			if err := s.commandOnly(ctx, "systemctl", "disable", "--now", unit.name); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("disable orphaned %s: %w", unit.name, err))
			}
		} else if !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	if err := s.removeGatewayTables(ctx); err != nil {
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove orphaned Gateway nftables: %w", err))
	}
	rules, err := s.command(ctx, "ip", "-N", "-4", "rule", "show")
	if err != nil {
		cleanupErr = errors.Join(cleanupErr, err)
	} else {
		for _, priority := range gatewayOwnedRulePriorities(rules) {
			if err := s.commandOnly(ctx, "ip", "-4", "rule", "del", "priority", priority, "from", "all", "fwmark", "0x40/0xc0", "table", "100"); err != nil {
				cleanupErr = errors.Join(cleanupErr, err)
			}
		}
	}
	routes, routeErr := s.command(ctx, "ip", "-4", "route", "show", "table", "100")
	if routeErr != nil && !strings.Contains(routes, "FIB table does not exist") {
		cleanupErr = errors.Join(cleanupErr, routeErr)
	} else if gatewayHasOwnedRoute(routes) {
		if err := s.commandOnly(ctx, "ip", "-4", "route", "del", "local", "default", "dev", "lo", "table", "100"); err != nil {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	if !removeFiles {
		return cleanupErr
	}
	// rp_filter has its own self-describing snapshot and can therefore still
	// be restored safely even when gateway.env (which held ip_forward) is lost.
	if _, err := os.Stat(paths.rpFilter); err == nil {
		if err := s.restoreGatewayRPFilter(ctx); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("restore orphaned Gateway rp_filter snapshot: %w", err))
		} else if err := os.Remove(paths.rpFilter); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	} else if !os.IsNotExist(err) {
		cleanupErr = errors.Join(cleanupErr, err)
	}
	for _, path := range []string{paths.firewallUnit, paths.routingUnit, paths.restoreScript, paths.nft} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			cleanupErr = errors.Join(cleanupErr, err)
		}
	}
	if err := s.commandOnly(ctx, "systemctl", "daemon-reload"); err != nil {
		cleanupErr = errors.Join(cleanupErr, err)
	}
	return cleanupErr
}

func (s *GatewayNetworkService) removeGatewayTables(ctx context.Context) error {
	tables, err := s.command(ctx, "nft", "list", "tables")
	if err != nil {
		return err
	}
	var batch strings.Builder
	for _, table := range []struct{ family, name string }{{"inet", "xui_gateway"}, {"ip", "xui_gateway_nat"}} {
		if gatewayTablePresent(tables, table.family, table.name) {
			fmt.Fprintf(&batch, "delete table %s %s\n", table.family, table.name)
		}
	}
	if batch.Len() == 0 {
		return nil
	}
	return s.commandInput(ctx, batch.String(), "nft", "-f", "-")
}
func (s *GatewayNetworkService) Disable(ctx context.Context) error {
	state, err := s.loadGatewayNetworkState()
	if os.IsNotExist(err) {
		orphaned, inspectErr := s.gatewayOrphaned(ctx)
		if inspectErr != nil {
			return fmt.Errorf("inspect Gateway networking without recovery state: %w", inspectErr)
		}
		if !orphaned {
			return nil
		}
		return s.cleanupGatewayWithoutState(ctx, true)
	}
	if err != nil {
		return fmt.Errorf("read Gateway recovery state: %w", err)
	}
	if err := s.Suspend(ctx); err != nil {
		return err
	}
	for _, unit := range []struct{ path, name string }{{s.networkPaths().firewallUnit, "xui-gateway-firewall.service"}, {s.networkPaths().routingUnit, "xui-gateway-routing.service"}} {
		if _, statErr := os.Stat(unit.path); statErr == nil {
			if err := s.commandOnly(ctx, "systemctl", "disable", "--now", unit.name); err != nil {
				return err
			}
		}
	}
	rules, err := s.command(ctx, "ip", "-N", "-4", "rule", "show")
	if err != nil {
		return err
	}
	for _, priority := range gatewayOwnedRulePriorities(rules) {
		if err := s.commandOnly(ctx, "ip", "-4", "rule", "del", "priority", priority, "from", "all", "fwmark", "0x40/0xc0", "table", "100"); err != nil {
			return err
		}
	}
	routes, err := s.command(ctx, "ip", "-4", "route", "show", "table", "100")
	if err != nil && !strings.Contains(routes, "FIB table does not exist") {
		return err
	}
	if gatewayHasOwnedRoute(routes) {
		if err := s.commandOnly(ctx, "ip", "-4", "route", "del", "local", "default", "dev", "lo", "table", "100"); err != nil {
			return err
		}
	}
	current, err := s.command(ctx, "sysctl", "-n", "net.ipv4.ip_forward")
	if err != nil {
		return err
	}
	if strings.TrimSpace(current) != state.OldForwarding {
		if _, err := s.command(ctx, "sysctl", "-w", "net.ipv4.ip_forward="+state.OldForwarding); err != nil {
			return fmt.Errorf("restore IPv4 forwarding: %w", err)
		}
	}
	if err := s.restoreGatewayRPFilter(ctx); err != nil {
		return err
	}
	for _, path := range []string{s.networkPaths().firewallUnit, s.networkPaths().routingUnit, s.networkPaths().restoreScript, s.networkPaths().nft} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := s.commandOnly(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	if err := os.Remove(s.networkPaths().rpFilter); err != nil && !os.IsNotExist(err) {
		return err
	}
	return os.Remove(s.networkPaths().state)
}

func (s *GatewayNetworkService) applyGatewayNetwork(ctx context.Context, cfg GatewayNetworkConfig, lanNetwork, oldForwarding string, persistState bool) (err error) {
	if err = os.MkdirAll(filepath.Dir(s.networkPaths().state), 0o755); err != nil {
		return fmt.Errorf("create gateway state directory: %w", err)
	}

	// Validate a replacement transaction before changing the live firewall.
	batch := gatewayNFTRules(lanNetwork, cfg.WANInterface, cfg.LANInterface)
	tables, listErr := s.command(ctx, "nft", "list", "tables")
	if listErr != nil {
		return listErr
	}
	for _, table := range []struct{ family, name string }{{"inet", "xui_gateway"}, {"ip", "xui_gateway_nat"}} {
		if gatewayTablePresent(tables, table.family, table.name) {
			if persistState {
				return fmt.Errorf("Gateway firewall exists without recovery state; clean it up first")
			}
			batch = "delete table " + table.family + " " + table.name + "\n" + batch
		}
	}
	if err = s.commandInput(ctx, batch, "nft", "-c", "-f", "-"); err != nil {
		return fmt.Errorf("validate Gateway nftables: %w", err)
	}
	if persistState {
		if _, statErr := os.Stat(s.networkPaths().rpFilter); statErr == nil {
			return fmt.Errorf("orphaned Gateway rp_filter snapshot requires recovery")
		} else if !os.IsNotExist(statErr) {
			return statErr
		}
		if err = s.saveGatewayNetworkState(gatewayNetworkState{OldForwarding: oldForwarding, Config: cfg, LANNetwork: lanNetwork}); err != nil {
			return err
		}
	}
	firewallApplied := false
	defer func() {
		if err == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		var cleanupErr error
		switch {
		case persistState:
			cleanupErr = s.Disable(cleanupCtx)
		case firewallApplied:
			// A repair/resume already had durable recovery state. If it fails
			// after replacing the live firewall, fail closed instead of leaving
			// interception active while reporting an unsuccessful operation.
			cleanupErr = s.Suspend(cleanupCtx)
		default:
			return
		}
		if cleanupErr != nil {
			err = errors.Join(err, fmt.Errorf("Gateway rollback failed: %w", cleanupErr))
		}
	}()

	if err = s.captureGatewayRPFilter(ctx, cfg.LANInterface); err != nil {
		return err
	}
	currentForwarding, readErr := s.command(ctx, "sysctl", "-n", "net.ipv4.ip_forward")
	if readErr != nil {
		return readErr
	}
	if strings.TrimSpace(currentForwarding) != "1" {
		if _, err = s.command(ctx, "sysctl", "-w", "net.ipv4.ip_forward=1"); err != nil {
			return fmt.Errorf("enable IPv4 forwarding: %w", err)
		}
	}

	if err = s.configureGatewayRPFilter(ctx, cfg.LANInterface); err != nil {
		return err
	}
	routes, routeErr := s.command(ctx, "ip", "-N", "-4", "route", "show", "table", "100")
	if routeErr != nil && !strings.Contains(routes, "FIB table does not exist") {
		return routeErr
	}
	if strings.TrimSpace(routes) != "" && routeErr == nil && !gatewayOwnedRoutes(routes) {
		return fmt.Errorf("Gateway routing table contains a foreign route")
	}
	rules, ruleErr := s.command(ctx, "ip", "-N", "-4", "rule", "show")
	if ruleErr != nil {
		return ruleErr
	}
	if gatewayForeignRule(rules) {
		return fmt.Errorf("Gateway routing table is referenced by a foreign rule")
	}
	count := gatewayPolicyRuleCount(rules)
	if count > 1 {
		for _, priority := range gatewayOwnedRulePriorities(rules) {
			if _, err = s.command(ctx, "ip", "-4", "rule", "del", "priority", priority, "from", "all", "fwmark", "0x40/0xc0", "table", "100"); err != nil {
				return err
			}
		}
		count = 0
	}
	if count == 0 {
		if _, err = s.command(ctx, "ip", "-4", "rule", "add", "fwmark", "0x40/0xc0", "table", "100"); err != nil {
			return fmt.Errorf("add Gateway policy rule: %w", err)
		}
	}
	if _, err = s.command(ctx, "ip", "-4", "route", "replace", "local", "default", "dev", "lo", "table", "100"); err != nil {
		return fmt.Errorf("add Gateway policy route: %w", err)
	}

	nftRules := gatewayNFTRules(lanNetwork, cfg.WANInterface, cfg.LANInterface)
	if err = s.commandInput(ctx, batch, "nft", "-f", "-"); err != nil {
		return fmt.Errorf("apply Gateway nftables rules: %w", err)
	}
	firewallApplied = true
	if err = gatewayAtomicWrite(s.networkPaths().nft, []byte(nftRules), 0o600); err != nil {
		return fmt.Errorf("save Gateway nftables rules: %w", err)
	}
	if err = s.installGatewayNetworkUnits(ctx); err != nil {
		return err
	}
	return s.saveGatewayNetworkState(gatewayNetworkState{OldForwarding: oldForwarding, Config: cfg, LANNetwork: lanNetwork, DesiredActive: true})
}

func normalizeGatewayNetworkConfig(cfg GatewayNetworkConfig) (GatewayNetworkConfig, string, error) {
	cfg.LANInterface = strings.TrimSpace(cfg.LANInterface)
	cfg.LANIP = strings.TrimSpace(cfg.LANIP)
	cfg.WANInterface = strings.TrimSpace(cfg.WANInterface)
	if cfg.LANPrefix < 1 || cfg.LANPrefix > 32 {
		return cfg, "", fmt.Errorf("LAN prefix must be between 1 and 32")
	}
	if !gatewayInterfaceName.MatchString(cfg.LANInterface) || (cfg.WANInterface != "" && !gatewayInterfaceName.MatchString(cfg.WANInterface)) {
		return cfg, "", fmt.Errorf("invalid Gateway interface name")
	}
	if cfg.LANInterface == "" {
		return cfg, "", fmt.Errorf("LAN interface is required")
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
		if cfg.WANInterface == cfg.LANInterface {
			return cfg, "", fmt.Errorf("LAN and WAN interfaces must be different")
		}
	}
	cfg.LANIP = ip.String()
	return cfg, network.String(), nil
}

func (s *GatewayNetworkService) saveGatewayNetworkState(state gatewayNetworkState) error {
	active := 0
	if state.DesiredActive {
		active = 1
	}
	content := fmt.Sprintf("NETWORK_ENABLED=%d\nIP_FORWARD_OLD=%s\nLAN_IF=%s\nLAN_IP=%s\nLAN_PREFIX=%d\nLAN_NETWORK=%s\nWAN_IF=%s\n",
		active,
		state.OldForwarding, state.Config.LANInterface, state.Config.LANIP, state.Config.LANPrefix, state.LANNetwork, state.Config.WANInterface)
	return gatewayAtomicWrite(s.networkPaths().state, []byte(content), 0600)
}

func (s *GatewayNetworkService) loadGatewayNetworkState() (gatewayNetworkState, error) {
	f, err := os.Open(s.networkPaths().state)
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
		if !ok {
			return gatewayNetworkState{}, fmt.Errorf("malformed Gateway state line")
		}
		key = strings.TrimSpace(key)
		if _, exists := values[key]; exists {
			return gatewayNetworkState{}, fmt.Errorf("duplicate Gateway state key %q", key)
		}
		values[key] = strings.TrimSpace(value)
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
		DesiredActive: values["NETWORK_ENABLED"] != "0",
		Legacy:        values["NETWORK_ENABLED"] == "",
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
	if values["NETWORK_ENABLED"] != "" && values["NETWORK_ENABLED"] != "0" && values["NETWORK_ENABLED"] != "1" {
		return gatewayNetworkState{}, fmt.Errorf("invalid Gateway NETWORK_ENABLED")
	}
	if err := validateGatewayStoredConfig(state); err != nil {
		return gatewayNetworkState{}, err
	}
	return state, nil
}

func (s *GatewayNetworkService) installGatewayNetworkUnits(ctx context.Context) error {
	routingUnit, firewallUnit, script, err := s.gatewayNetworkUnits()
	if err != nil {
		return err
	}
	for _, file := range []struct {
		path, content string
		mode          os.FileMode
	}{
		{s.networkPaths().routingUnit, routingUnit, 0644}, {s.networkPaths().firewallUnit, firewallUnit, 0644}, {s.networkPaths().restoreScript, script, 0700},
	} {
		if err := gatewayAtomicWrite(file.path, []byte(file.content), file.mode); err != nil {
			return err
		}
	}
	if _, err := s.command(ctx, "systemctl", "daemon-reload"); err != nil {
		return err
	}
	_, err = s.command(ctx, "systemctl", "enable", "xui-gateway-routing.service", "xui-gateway-firewall.service")
	return err
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
    set bypass {
        type ipv4_addr; flags interval; auto-merge;
        elements = { 0.0.0.0/8, 10.0.0.0/8, 100.64.0.0/10, 127.0.0.0/8, 169.254.0.0/16, 172.16.0.0/12, 192.0.0.0/24, 192.0.2.0/24, 192.88.99.0/24, 192.168.0.0/16, 198.51.100.0/24, 203.0.113.0/24, 224.0.0.0/3, %s }
    }
    chain prerouting {
        type filter hook prerouting priority mangle - 5; policy accept;
        meta nfproto != ipv4 return
        iifname != %q return
        ip saddr != %s return
        meta l4proto != { tcp, udp } return
        fib daddr type local return
        ip daddr @bypass return
        meta mark & 0x80 == 0x80 return
        meta mark set (meta mark & 0xffffff3f) | 0x40
        counter tproxy ip to 127.0.0.1:52345 accept comment "3x-ui gateway v2"
    }
}%s`, lanNetwork, lanInterface, lanNetwork, nat)
}

func runGatewayCommand(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
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
	cmd.Env = append(os.Environ(), "LC_ALL=C")
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

func (s *GatewayNetworkService) validateGatewayRPSnapshot() error {
	data, err := os.ReadFile(s.networkPaths().rpFilter)
	if err != nil {
		return err
	}
	_, err = parseGatewayRPSnapshot(string(data))
	return err
}

func gatewayTableInUse(rules string) bool        { return gatewayTableRule.MatchString(rules) }
func gatewayPolicyRulePresent(rules string) bool { return gatewayPolicyRule.MatchString(rules) }

// Strict reverse-path filtering can discard packets routed to the TPROXY
// listener. Snapshot all interfaces before ip_forward resets IPv4 defaults.
func (s *GatewayNetworkService) captureGatewayRPFilter(ctx context.Context, iface string) error {
	if data, err := os.ReadFile(s.networkPaths().rpFilter); err == nil {
		_, err := parseGatewayRPSnapshot(string(data))
		return err
	} else if !os.IsNotExist(err) {
		return err
	}
	getInterfaces := s.interfaces
	if getInterfaces == nil {
		getInterfaces = net.Interfaces
	}
	interfaces, err := getInterfaces()
	if err != nil {
		return err
	}
	names := map[string]bool{"all": true, "default": true, iface: true}
	for _, item := range interfaces {
		names[item.Name] = true
	}
	keys := make([]string, 0, len(names))
	for name := range names {
		keys = append(keys, "net/ipv4/conf/"+name+"/rp_filter")
	}
	sort.Strings(keys)
	var snapshot strings.Builder
	for _, key := range keys {
		value, err := s.command(ctx, "sysctl", "-n", key)
		if err != nil {
			return err
		}
		value = strings.TrimSpace(value)
		if value != "0" && value != "1" && value != "2" {
			return fmt.Errorf("invalid rp_filter value %q", value)
		}
		fmt.Fprintf(&snapshot, "%s %s\n", key, value)
	}
	return gatewayAtomicWrite(s.networkPaths().rpFilter, []byte(snapshot.String()), 0600)
}
func (s *GatewayNetworkService) configureGatewayRPFilter(ctx context.Context, iface string) error {
	if err := s.captureGatewayRPFilter(ctx, iface); err != nil {
		return err
	}
	return s.applyGatewayRPFilter(ctx, false, iface)
}
func (s *GatewayNetworkService) restoreGatewayRPFilter(ctx context.Context) error {
	return s.applyGatewayRPFilter(ctx, true, "")
}
func parseGatewayRPSnapshot(data string) (map[string]string, error) {
	values := map[string]string{}
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || !gatewayRPKey.MatchString(fields[0]) || (fields[1] != "0" && fields[1] != "1" && fields[1] != "2") {
			return nil, fmt.Errorf("invalid Gateway rp_filter snapshot")
		}
		if _, exists := values[fields[0]]; exists {
			return nil, fmt.Errorf("duplicate Gateway rp_filter snapshot key")
		}
		values[fields[0]] = fields[1]
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("empty Gateway rp_filter snapshot")
	}
	return values, nil
}
func (s *GatewayNetworkService) applyGatewayRPFilter(ctx context.Context, restore bool, iface string) error {
	data, err := os.ReadFile(s.networkPaths().rpFilter)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	values, err := parseGatewayRPSnapshot(string(data))
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if s.run == nil {
			if _, err := os.Stat("/proc/sys/" + key); os.IsNotExist(err) {
				continue
			}
		}
		value := values[key]
		if !restore && (key == "net/ipv4/conf/all/rp_filter" || key == "net/ipv4/conf/"+iface+"/rp_filter") {
			value = "0"
		}
		if _, err := s.command(ctx, "sysctl", "-w", key+"="+value); err != nil {
			return err
		}
	}
	return nil
}
