package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

const (
	firewallStateSettingKey = "firewallManagedState"
	firewallNftTable        = "xui_firewall"
	firewallIptablesChain   = "XUI-FIREWALL"
	firewalldServiceName    = "x-ui-managed"
	ufwManagedComment       = "x-ui-managed"
)

// FirewallPortRule is a port the panel keeps reachable while managed firewall
// mode is enabled. Protocol is tcp, udp or both.
type FirewallPortRule struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Label    string `json:"label,omitempty"`
	Source   string `json:"source,omitempty"`
}

type firewallManagedState struct {
	Enabled     bool               `json:"enabled"`
	AutoSync    bool               `json:"autoSync"`
	Backend     string             `json:"backend,omitempty"`
	ManualRules []FirewallPortRule `json:"manualRules,omitempty"`
}

// FirewallStatus is returned to the web UI.
type FirewallStatus struct {
	Available     bool               `json:"available"`
	Enabled       bool               `json:"enabled"`
	AutoSync      bool               `json:"autoSync"`
	Backend       string             `json:"backend"`
	External      bool               `json:"external"`
	Rules         []FirewallPortRule `json:"rules"`
	ManualRules   []FirewallPortRule `json:"manualRules"`
	BackendNotice string             `json:"backendNotice,omitempty"`
}

type FirewallService struct {
	settingService SettingService
}

var (
	firewallSyncMu       sync.Mutex
	firewallReconcileOnce sync.Once
	firewallSyncTrigger   = make(chan struct{}, 1)
)

func normalizeFirewallProtocol(protocol string) (string, error) {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	switch protocol {
	case "tcp", "udp", "both":
		return protocol, nil
	default:
		return "", fmt.Errorf("unsupported firewall protocol %q", protocol)
	}
}

func normalizeFirewallRule(rule FirewallPortRule) (FirewallPortRule, error) {
	if rule.Port < 1 || rule.Port > 65535 {
		return FirewallPortRule{}, fmt.Errorf("port must be between 1 and 65535")
	}
	protocol, err := normalizeFirewallProtocol(rule.Protocol)
	if err != nil {
		return FirewallPortRule{}, err
	}
	rule.Protocol = protocol
	rule.Label = strings.TrimSpace(rule.Label)
	if len(rule.Label) > 120 {
		rule.Label = rule.Label[:120]
	}
	return rule, nil
}

func (s *FirewallService) loadState() (firewallManagedState, error) {
	state := firewallManagedState{AutoSync: true}
	setting, err := s.settingService.getSetting(firewallStateSettingKey)
	if database.IsNotFound(err) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if strings.TrimSpace(setting.Value) == "" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(setting.Value), &state); err != nil {
		return firewallManagedState{AutoSync: true}, fmt.Errorf("decode managed firewall settings: %w", err)
	}
	// Old/hand-edited state may omit autoSync. Managed firewall is intended to
	// be automatic by default, so only an explicit false in a valid saved state
	// disables reconciliation.
	return state, nil
}

func (s *FirewallService) saveState(state firewallManagedState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.settingService.saveSetting(firewallStateSettingKey, string(data))
}

func commandOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func isFirewalldActive(ctx context.Context) bool {
	if !commandExists("firewall-cmd") {
		return false
	}
	out, err := commandOutput(ctx, "firewall-cmd", "--state")
	return err == nil && strings.EqualFold(strings.TrimSpace(out), "running")
}

func isUFWActive(ctx context.Context) bool {
	if !commandExists("ufw") {
		return false
	}
	out, err := commandOutput(ctx, "ufw", "status")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(out), "status: active")
}

func detectFirewallBackend(ctx context.Context) string {
	// If an administrator already uses a distribution firewall, integrate with
	// it instead of bypassing it with another independent ruleset.
	if isFirewalldActive(ctx) {
		return "firewalld"
	}
	if isUFWActive(ctx) {
		return "ufw"
	}
	if commandExists("nft") {
		return "nftables"
	}
	if commandExists("iptables") {
		return "iptables"
	}
	return ""
}

func backendIsExternal(name string) bool {
	return name == "firewalld" || name == "ufw"
}

func (s *FirewallService) Status() (*FirewallStatus, error) {
	state, err := s.loadState()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	backend := state.Backend
	if backend == "" || !state.Enabled {
		backend = detectFirewallBackend(ctx)
	}
	rules, err := s.desiredRules(ctx, state.ManualRules)
	if err != nil {
		return nil, err
	}
	status := &FirewallStatus{
		Available:   backend != "",
		Enabled:     state.Enabled,
		AutoSync:    state.AutoSync,
		Backend:     backend,
		External:    backendIsExternal(backend),
		Rules:       rules,
		ManualRules: append([]FirewallPortRule(nil), state.ManualRules...),
	}
	if status.External {
		status.BackendNotice = "The existing system firewall stays enabled; x-ui manages only its own ports."
	}
	return status, nil
}

func (s *FirewallService) SetEnabled(enabled bool) error {
	firewallSyncMu.Lock()
	defer firewallSyncMu.Unlock()
	state, err := s.loadState()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if !enabled {
		backend := state.Backend
		if backend == "" {
			backend = detectFirewallBackend(ctx)
		}
		if backend != "" {
			if err := s.disableBackend(ctx, backend); err != nil {
				return err
			}
		}
		state.Enabled = false
		state.Backend = ""
		return s.saveState(state)
	}

	backend := detectFirewallBackend(ctx)
	if backend == "" {
		return fmt.Errorf("no supported firewall backend found (firewalld, ufw, nftables or iptables)")
	}
	rules, err := s.desiredRules(ctx, state.ManualRules)
	if err != nil {
		return err
	}
	if err := s.applyBackend(ctx, backend, rules); err != nil {
		return err
	}
	state.Enabled = true
	state.Backend = backend
	if err := s.saveState(state); err != nil {
		_ = s.disableBackend(ctx, backend)
		return err
	}
	return nil
}

func (s *FirewallService) SetAutoSync(auto bool) error {
	state, err := s.loadState()
	if err != nil {
		return err
	}
	state.AutoSync = auto
	if err := s.saveState(state); err != nil {
		return err
	}
	if auto && state.Enabled {
		TriggerFirewallSync()
	}
	return nil
}

func (s *FirewallService) AddManualRule(rule FirewallPortRule) error {
	rule.Source = "manual"
	normalized, err := normalizeFirewallRule(rule)
	if err != nil {
		return err
	}
	state, err := s.loadState()
	if err != nil {
		return err
	}
	for _, current := range state.ManualRules {
		if current.Port == normalized.Port && current.Protocol == normalized.Protocol {
			return fmt.Errorf("this manual firewall rule already exists")
		}
	}
	state.ManualRules = append(state.ManualRules, normalized)
	if err := s.saveState(state); err != nil {
		return err
	}
	if state.Enabled {
		TriggerFirewallSync()
	}
	return nil
}

func (s *FirewallService) DeleteManualRule(port int, protocol string) error {
	protocol, err := normalizeFirewallProtocol(protocol)
	if err != nil {
		return err
	}
	state, err := s.loadState()
	if err != nil {
		return err
	}
	next := state.ManualRules[:0]
	for _, rule := range state.ManualRules {
		if rule.Port == port && rule.Protocol == protocol {
			continue
		}
		next = append(next, rule)
	}
	state.ManualRules = next
	if err := s.saveState(state); err != nil {
		return err
	}
	if state.Enabled {
		TriggerFirewallSync()
	}
	return nil
}

func (s *FirewallService) Sync() error {
	firewallSyncMu.Lock()
	defer firewallSyncMu.Unlock()
	state, err := s.loadState()
	if err != nil {
		return err
	}
	if !state.Enabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	backend := detectFirewallBackend(ctx)
	if backend == "" {
		return fmt.Errorf("no supported firewall backend found")
	}
	if state.Backend != "" && state.Backend != backend {
		_ = s.disableBackend(ctx, state.Backend)
	}
	rules, err := s.desiredRules(ctx, state.ManualRules)
	if err != nil {
		return err
	}
	if err := s.applyBackend(ctx, backend, rules); err != nil {
		return err
	}
	if state.Backend != backend {
		state.Backend = backend
		return s.saveState(state)
	}
	return nil
}

func (s *FirewallService) SyncIfEnabled() error {
	state, err := s.loadState()
	if err != nil {
		return err
	}
	if !state.Enabled || !state.AutoSync {
		return nil
	}
	return s.Sync()
}

func (s *FirewallService) desiredRules(ctx context.Context, manual []FirewallPortRule) ([]FirewallPortRule, error) {
	rules := make([]FirewallPortRule, 0, 16+len(manual))
	if port, err := s.settingService.GetPort(); err == nil && port > 0 {
		rules = append(rules, FirewallPortRule{Port: port, Protocol: "tcp", Label: "3x-ui panel", Source: "panel"})
	}
	if enabled, err := s.settingService.GetSubEnable(); err == nil && enabled {
		if port, err := s.settingService.GetSubPort(); err == nil && port > 0 {
			rules = append(rules, FirewallPortRule{Port: port, Protocol: "tcp", Label: "Subscriptions", Source: "subscription"})
		}
	}

	// Keep standard SSH reachable even when process discovery is unavailable.
	// Any non-standard sshd listeners discovered by ss are added as well.
	rules = append(rules, FirewallPortRule{Port: 22, Protocol: "tcp", Label: "SSH safety", Source: "system"})
	for _, port := range discoverSSHPorts(ctx) {
		if port != 22 {
			rules = append(rules, FirewallPortRule{Port: port, Protocol: "tcp", Label: "SSH", Source: "system"})
		}
	}

	var inbounds []model.Inbound
	if err := database.GetDB().Select("id", "remark", "port", "protocol", "stream_settings", "node_id", "enable").
		Where("enable = ?", true).
		Where("node_id IS NULL").
		Find(&inbounds).Error; err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		if inbound.Port < 1 || inbound.Port > 65535 {
			continue
		}
		label := strings.TrimSpace(inbound.Remark)
		if label == "" {
			label = string(inbound.Protocol)
		}
		rules = append(rules, FirewallPortRule{
			Port:     inbound.Port,
			Protocol: inboundFirewallProtocol(inbound),
			Label:    label,
			Source:   "inbound",
		})
	}
	for _, rule := range manual {
		normalized, err := normalizeFirewallRule(rule)
		if err != nil {
			continue
		}
		normalized.Source = "manual"
		rules = append(rules, normalized)
	}
	return dedupeFirewallRules(rules), nil
}

func inboundFirewallProtocol(inbound model.Inbound) string {
	var stream struct {
		Network string `json:"network"`
	}
	if json.Unmarshal([]byte(inbound.StreamSettings), &stream) == nil {
		switch strings.ToLower(stream.Network) {
		case "kcp", "mkcp", "quic", "hysteria":
			return "udp"
		case "tcp", "ws", "http", "h2", "grpc", "xhttp", "splithttp":
			return "tcp"
		}
	}
	switch inbound.Protocol {
	case model.WireGuard, model.Hysteria, model.AmneziaWG, model.TUIC:
		return "udp"
	case model.VMESS, model.VLESS, model.HTTP, model.Trojan, model.MTProto, model.NaiveProxy, model.AnyTLS, model.ShadowTLS, model.TrustTunnel:
		return "tcp"
	default:
		// Shadowsocks, mixed and several sidecars may serve both transports on
		// the same port; opening both avoids a protocol-specific false negative.
		return "both"
	}
}

func dedupeFirewallRules(rules []FirewallPortRule) []FirewallPortRule {
	seen := make(map[string]struct{}, len(rules))
	out := make([]FirewallPortRule, 0, len(rules))
	for _, rule := range rules {
		normalized, err := normalizeFirewallRule(rule)
		if err != nil {
			continue
		}
		key := strconv.Itoa(normalized.Port) + "/" + normalized.Protocol
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, normalized)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Port == out[j].Port {
			return out[i].Protocol < out[j].Protocol
		}
		return out[i].Port < out[j].Port
	})
	return out
}

func discoverSSHPorts(ctx context.Context) []int {
	if !commandExists("ss") {
		return nil
	}
	out, err := commandOutput(ctx, "ss", "-H", "-ltnp")
	if err != nil {
		return nil
	}
	ports := map[int]struct{}{}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(strings.ToLower(line), "sshd") {
			continue
		}
		fields := strings.Fields(line)
		for _, field := range fields {
			idx := strings.LastIndex(field, ":")
			if idx < 0 || idx == len(field)-1 {
				continue
			}
			p, err := strconv.Atoi(strings.Trim(field[idx+1:], "[]"))
			if err == nil && p > 0 && p <= 65535 {
				ports[p] = struct{}{}
			}
		}
	}
	outPorts := make([]int, 0, len(ports))
	for p := range ports {
		outPorts = append(outPorts, p)
	}
	sort.Ints(outPorts)
	return outPorts
}

func expandedPorts(rules []FirewallPortRule) (tcp []int, udp []int) {
	tcpSet := map[int]struct{}{}
	udpSet := map[int]struct{}{}
	for _, rule := range rules {
		switch rule.Protocol {
		case "tcp":
			tcpSet[rule.Port] = struct{}{}
		case "udp":
			udpSet[rule.Port] = struct{}{}
		case "both":
			tcpSet[rule.Port] = struct{}{}
			udpSet[rule.Port] = struct{}{}
		}
	}
	for p := range tcpSet {
		tcp = append(tcp, p)
	}
	for p := range udpSet {
		udp = append(udp, p)
	}
	sort.Ints(tcp)
	sort.Ints(udp)
	return tcp, udp
}

func (s *FirewallService) applyBackend(ctx context.Context, backend string, rules []FirewallPortRule) error {
	tcp, udp := expandedPorts(rules)
	switch backend {
	case "nftables":
		return applyNftables(ctx, tcp, udp)
	case "iptables":
		return applyIptables(ctx, tcp, udp)
	case "ufw":
		return applyUFW(ctx, tcp, udp)
	case "firewalld":
		return applyFirewalld(ctx, tcp, udp)
	default:
		return fmt.Errorf("unsupported firewall backend %q", backend)
	}
}

func (s *FirewallService) disableBackend(ctx context.Context, backend string) error {
	switch backend {
	case "nftables":
		if !commandExists("nft") {
			return nil
		}
		_, _ = commandOutput(ctx, "nft", "delete", "table", "inet", firewallNftTable)
		return nil
	case "iptables":
		return removeIptables(ctx)
	case "ufw":
		return clearUFWManaged(ctx)
	case "firewalld":
		return disableFirewalldManaged(ctx)
	default:
		return nil
	}
}

func applyNftables(ctx context.Context, tcp, udp []int) error {
	_, _ = commandOutput(ctx, "nft", "delete", "table", "inet", firewallNftTable)
	var b strings.Builder
	b.WriteString("table inet " + firewallNftTable + " {\n")
	b.WriteString("  chain input {\n")
	b.WriteString("    type filter hook input priority -10; policy drop;\n")
	b.WriteString("    ct state established,related accept\n")
	b.WriteString("    iifname \"lo\" accept\n")
	b.WriteString("    ip protocol icmp accept\n")
	b.WriteString("    ip6 nexthdr ipv6-icmp accept\n")
	if len(tcp) > 0 {
		b.WriteString("    tcp dport { " + joinPorts(tcp) + " } accept\n")
	}
	if len(udp) > 0 {
		b.WriteString("    udp dport { " + joinPorts(udp) + " } accept\n")
	}
	b.WriteString("  }\n}\n")
	cmd := exec.CommandContext(ctx, "nft", "-f", "-")
	cmd.Stdin = strings.NewReader(b.String())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("apply nftables rules: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func joinPorts(ports []int) string {
	values := make([]string, 0, len(ports))
	for _, p := range ports {
		values = append(values, strconv.Itoa(p))
	}
	return strings.Join(values, ", ")
}

func applyIptables(ctx context.Context, tcp, udp []int) error {
	if err := applyIptablesBinary(ctx, "iptables", "icmp", tcp, udp); err != nil {
		return err
	}
	if commandExists("ip6tables") {
		if err := applyIptablesBinary(ctx, "ip6tables", "ipv6-icmp", tcp, udp); err != nil {
			return err
		}
	}
	return nil
}

func applyIptablesBinary(ctx context.Context, binary, icmpProto string, tcp, udp []int) error {
	_, _ = commandOutput(ctx, binary, "-N", firewallIptablesChain)
	if _, err := commandOutput(ctx, binary, "-F", firewallIptablesChain); err != nil {
		return err
	}
	commands := [][]string{
		{"-A", firewallIptablesChain, "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"},
		{"-A", firewallIptablesChain, "-i", "lo", "-j", "ACCEPT"},
		{"-A", firewallIptablesChain, "-p", icmpProto, "-j", "ACCEPT"},
	}
	for _, p := range tcp {
		commands = append(commands, []string{"-A", firewallIptablesChain, "-p", "tcp", "--dport", strconv.Itoa(p), "-j", "ACCEPT"})
	}
	for _, p := range udp {
		commands = append(commands, []string{"-A", firewallIptablesChain, "-p", "udp", "--dport", strconv.Itoa(p), "-j", "ACCEPT"})
	}
	commands = append(commands, []string{"-A", firewallIptablesChain, "-j", "DROP"})
	for _, args := range commands {
		if _, err := commandOutput(ctx, binary, args...); err != nil {
			return err
		}
	}
	if _, err := commandOutput(ctx, binary, "-C", "INPUT", "-j", firewallIptablesChain); err != nil {
		if _, err := commandOutput(ctx, binary, "-I", "INPUT", "1", "-j", firewallIptablesChain); err != nil {
			return err
		}
	}
	return nil
}

func removeIptables(ctx context.Context) error {
	for _, binary := range []string{"iptables", "ip6tables"} {
		if !commandExists(binary) {
			continue
		}
		for {
			if _, err := commandOutput(ctx, binary, "-C", "INPUT", "-j", firewallIptablesChain); err != nil {
				break
			}
			_, _ = commandOutput(ctx, binary, "-D", "INPUT", "-j", firewallIptablesChain)
		}
		_, _ = commandOutput(ctx, binary, "-F", firewallIptablesChain)
		_, _ = commandOutput(ctx, binary, "-X", firewallIptablesChain)
	}
	return nil
}

var ufwRuleNumberRE = regexp.MustCompile(`^\[\s*(\d+)\].*#\s*` + ufwManagedComment + `(?:\s|$)`)

func clearUFWManaged(ctx context.Context) error {
	if !commandExists("ufw") {
		return nil
	}
	out, err := commandOutput(ctx, "ufw", "status", "numbered")
	if err != nil {
		return err
	}
	var numbers []int
	for _, line := range strings.Split(out, "\n") {
		match := ufwRuleNumberRE.FindStringSubmatch(strings.TrimSpace(line))
		if len(match) != 2 {
			continue
		}
		if n, err := strconv.Atoi(match[1]); err == nil {
			numbers = append(numbers, n)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(numbers)))
	for _, n := range numbers {
		if _, err := commandOutput(ctx, "ufw", "--force", "delete", strconv.Itoa(n)); err != nil {
			return err
		}
	}
	return nil
}

func applyUFW(ctx context.Context, tcp, udp []int) error {
	if err := clearUFWManaged(ctx); err != nil {
		return err
	}
	for _, p := range tcp {
		if _, err := commandOutput(ctx, "ufw", "allow", strconv.Itoa(p)+"/tcp", "comment", ufwManagedComment); err != nil {
			return err
		}
	}
	for _, p := range udp {
		if _, err := commandOutput(ctx, "ufw", "allow", strconv.Itoa(p)+"/udp", "comment", ufwManagedComment); err != nil {
			return err
		}
	}
	return nil
}

func applyFirewalld(ctx context.Context, tcp, udp []int) error {
	if !isFirewalldActive(ctx) {
		return fmt.Errorf("firewalld is not running")
	}
	// Dedicated firewalld service gives x-ui ownership without touching rules
	// created by the administrator or another application.
	if _, err := commandOutput(ctx, "firewall-cmd", "--permanent", "--new-service="+firewalldServiceName); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "already") && !strings.Contains(strings.ToLower(err.Error()), "name_conflict") {
			return err
		}
	}
	ports, _ := commandOutput(ctx, "firewall-cmd", "--permanent", "--service="+firewalldServiceName, "--get-ports")
	for _, item := range strings.Fields(ports) {
		_, _ = commandOutput(ctx, "firewall-cmd", "--permanent", "--service="+firewalldServiceName, "--remove-port="+item)
	}
	for _, p := range tcp {
		if _, err := commandOutput(ctx, "firewall-cmd", "--permanent", "--service="+firewalldServiceName, "--add-port="+strconv.Itoa(p)+"/tcp"); err != nil {
			return err
		}
	}
	for _, p := range udp {
		if _, err := commandOutput(ctx, "firewall-cmd", "--permanent", "--service="+firewalldServiceName, "--add-port="+strconv.Itoa(p)+"/udp"); err != nil {
			return err
		}
	}
	_, _ = commandOutput(ctx, "firewall-cmd", "--permanent", "--add-service="+firewalldServiceName)
	_, err := commandOutput(ctx, "firewall-cmd", "--reload")
	return err
}

func disableFirewalldManaged(ctx context.Context) error {
	if !commandExists("firewall-cmd") {
		return nil
	}
	_, _ = commandOutput(ctx, "firewall-cmd", "--permanent", "--remove-service="+firewalldServiceName)
	if isFirewalldActive(ctx) {
		_, err := commandOutput(ctx, "firewall-cmd", "--reload")
		return err
	}
	return nil
}

// StartFirewallReconciler starts one coalescing worker for immediate inbound
// change notifications plus a periodic drift repair. The worker is deliberately
// process-global because the panel creates only one database/runtime instance.
func StartFirewallReconciler() {
	firewallReconcileOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(15 * time.Second)
			defer ticker.Stop()
			service := &FirewallService{}
			for {
				select {
				case <-ticker.C:
				case <-firewallSyncTrigger:
				}
				if err := service.SyncIfEnabled(); err != nil {
					logger.Debug("managed firewall sync failed:", err)
				}
			}
		}()
		TriggerFirewallSync()
	})
}

func TriggerFirewallSync() {
	select {
	case firewallSyncTrigger <- struct{}{}:
	default:
	}
}
