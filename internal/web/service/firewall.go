package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database"
	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
)

const (
	firewallAutoSyncKey     = "firewallAutoSync"
	firewallManagedRulesKey = "firewallManagedRules"
	firewallManualRulesKey  = "firewallManualRules"
)

var firewallMu sync.Mutex

type FirewallRule struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Source   string `json:"source"`
	Label    string `json:"label"`
	Owned    bool   `json:"owned"`
	Exists   bool   `json:"exists"`
}

type FirewallManualRule struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}

type FirewallStatus struct {
	Supported   bool                 `json:"supported"`
	Backend     string               `json:"backend"`
	Enabled     bool                 `json:"enabled"`
	AutoSync    bool                 `json:"autoSync"`
	Rules       []FirewallRule       `json:"rules"`
	ManualRules []FirewallManualRule `json:"manualRules"`
	Message     string               `json:"message,omitempty"`
}

type FirewallService struct{}

type firewallBackend struct {
	name, binary, offline, zone string
	enabled                     func(context.Context) (bool, error)
}

func (s *FirewallService) GetStatus(ctx context.Context, safetyPort int) (FirewallStatus, error) {
	firewallMu.Lock()
	defer firewallMu.Unlock()
	return s.status(ctx, safetyPort)
}

func (s *FirewallService) SetEnabled(ctx context.Context, enabled bool, safetyPort int) (FirewallStatus, error) {
	firewallMu.Lock()
	defer firewallMu.Unlock()
	b, err := detectFirewallBackend(ctx)
	if err != nil {
		return FirewallStatus{}, err
	}
	if enabled {
		if err := s.sync(ctx, b, safetyPort); err != nil {
			return FirewallStatus{}, err
		}
		if err := setFirewallBackendEnabled(ctx, b, true); err != nil {
			return FirewallStatus{}, err
		}
		if err := s.sync(ctx, b, safetyPort); err != nil {
			return FirewallStatus{}, err
		}
	} else if err := setFirewallBackendEnabled(ctx, b, false); err != nil {
		return FirewallStatus{}, err
	}
	return s.status(ctx, safetyPort)
}

func (s *FirewallService) SetAutoSync(ctx context.Context, enabled bool, safetyPort int) (FirewallStatus, error) {
	firewallMu.Lock()
	defer firewallMu.Unlock()
	if err := (&SettingService{}).setBool(firewallAutoSyncKey, enabled); err != nil {
		return FirewallStatus{}, err
	}
	b, err := detectFirewallBackend(ctx)
	if err != nil {
		return FirewallStatus{}, err
	}
	if err := s.sync(ctx, b, safetyPort); err != nil {
		return FirewallStatus{}, err
	}
	return s.status(ctx, safetyPort)
}

func (s *FirewallService) Sync(ctx context.Context, safetyPort int) (FirewallStatus, error) {
	firewallMu.Lock()
	defer firewallMu.Unlock()
	b, err := detectFirewallBackend(ctx)
	if err != nil {
		return FirewallStatus{}, err
	}
	if err := s.sync(ctx, b, safetyPort); err != nil {
		return FirewallStatus{}, err
	}
	return s.status(ctx, safetyPort)
}

func (s *FirewallService) SyncIfEnabled(ctx context.Context) error {
	auto, err := firewallAutoSync()
	if err != nil || !auto {
		return err
	}
	b, err := detectFirewallBackend(ctx)
	if err != nil {
		return nil
	}
	on, err := b.enabled(ctx)
	if err != nil || !on {
		return err
	}
	_, err = s.Sync(ctx, 0)
	return err
}

func (s *FirewallService) AddManualRule(ctx context.Context, port int, protocol string, safetyPort int) (FirewallStatus, error) {
	return s.changeManualRule(ctx, port, protocol, safetyPort, true)
}

func (s *FirewallService) DeleteManualRule(ctx context.Context, port int, protocol string, safetyPort int) (FirewallStatus, error) {
	return s.changeManualRule(ctx, port, protocol, safetyPort, false)
}

func (s *FirewallService) changeManualRule(ctx context.Context, port int, protocol string, safetyPort int, add bool) (FirewallStatus, error) {
	firewallMu.Lock()
	defer firewallMu.Unlock()
	if port < 1 || port > 65535 {
		return FirewallStatus{}, fmt.Errorf("invalid port %d", port)
	}
	protos, err := normalizeFirewallProtocols(protocol)
	if err != nil {
		return FirewallStatus{}, err
	}
	manual, err := loadManualFirewallRules()
	if err != nil {
		return FirewallStatus{}, err
	}
	set := make(map[string]FirewallManualRule, len(manual)+2)
	for _, r := range manual {
		set[firewallRuleKey(r.Port, r.Protocol)] = r
	}
	for _, proto := range protos {
		key := firewallRuleKey(port, proto)
		if add {
			set[key] = FirewallManualRule{Port: port, Protocol: proto}
		} else {
			delete(set, key)
		}
	}
	manual = manual[:0]
	for _, r := range set {
		manual = append(manual, r)
	}
	sort.Slice(manual, func(i, j int) bool {
		return manual[i].Port < manual[j].Port || (manual[i].Port == manual[j].Port && manual[i].Protocol < manual[j].Protocol)
	})
	if err := saveFirewallJSON(firewallManualRulesKey, manual); err != nil {
		return FirewallStatus{}, err
	}
	b, err := detectFirewallBackend(ctx)
	if err != nil {
		return FirewallStatus{}, err
	}
	if err := s.sync(ctx, b, safetyPort); err != nil {
		return FirewallStatus{}, err
	}
	return s.status(ctx, safetyPort)
}

func (s *FirewallService) status(ctx context.Context, safetyPort int) (FirewallStatus, error) {
	auto, err := firewallAutoSync()
	if err != nil {
		return FirewallStatus{}, err
	}
	manual, err := loadManualFirewallRules()
	if err != nil {
		return FirewallStatus{}, err
	}
	st := FirewallStatus{AutoSync: auto, ManualRules: manual}
	b, err := detectFirewallBackend(ctx)
	if err != nil {
		st.Message = err.Error()
		return st, nil
	}
	st.Supported, st.Backend = true, b.name
	st.Enabled, _ = b.enabled(ctx)
	desired, err := s.desiredRules(auto, safetyPort)
	if err != nil {
		return FirewallStatus{}, err
	}
	existing, err := listFirewallRules(ctx, b)
	if err != nil {
		return FirewallStatus{}, err
	}
	managed, err := loadManagedFirewallRules()
	if err != nil {
		return FirewallStatus{}, err
	}
	owned := map[string]bool{}
	for _, r := range managed {
		owned[firewallRuleKey(r.Port, r.Protocol)] = r.Owned
	}
	for i := range desired {
		key := firewallRuleKey(desired[i].Port, desired[i].Protocol)
		desired[i].Exists, desired[i].Owned = existing[key], owned[key]
	}
	st.Rules = desired
	return st, nil
}

func (s *FirewallService) sync(ctx context.Context, b firewallBackend, safetyPort int) error {
	auto, err := firewallAutoSync()
	if err != nil {
		return err
	}
	desired, err := s.desiredRules(auto, safetyPort)
	if err != nil {
		return err
	}
	existing, err := listFirewallRules(ctx, b)
	if err != nil {
		return err
	}
	managed, err := loadManagedFirewallRules()
	if err != nil {
		return err
	}
	old := map[string]FirewallRule{}
	for _, r := range managed {
		old[firewallRuleKey(r.Port, r.Protocol)] = r
	}
	want := map[string]bool{}
	next := make([]FirewallRule, 0, len(desired))
	for _, r := range desired {
		key := firewallRuleKey(r.Port, r.Protocol)
		want[key] = true
		r.Owned = old[key].Owned
		if !existing[key] {
			if err := addFirewallRule(ctx, b, r); err != nil {
				return err
			}
			r.Owned = true
			existing[key] = true
		}
		r.Exists = true
		next = append(next, r)
	}
	for _, r := range managed {
		key := firewallRuleKey(r.Port, r.Protocol)
		if want[key] || !r.Owned || !existing[key] {
			continue
		}
		if err := deleteFirewallRule(ctx, b, r); err != nil {
			return err
		}
	}
	for i := range next {
		next[i].Exists = false
	}
	return saveFirewallJSON(firewallManagedRulesKey, next)
}

func (s *FirewallService) desiredRules(auto bool, safetyPort int) ([]FirewallRule, error) {
	m := map[string]FirewallRule{}
	add := func(port int, proto, source, label string) {
		if port < 1 || port > 65535 {
			return
		}
		key := firewallRuleKey(port, proto)
		if old, ok := m[key]; ok && firewallSourcePriority(source) <= firewallSourcePriority(old.Source) {
			return
		}
		m[key] = FirewallRule{Port: port, Protocol: proto, Source: source, Label: label}
	}
	settings := SettingService{}
	if p, err := settings.GetPort(); err == nil {
		add(p, "tcp", "panel", "Web panel")
	}
	if enabled, _ := settings.GetSubEnable(); enabled {
		if p, err := settings.GetSubPort(); err == nil {
			add(p, "tcp", "subscription", "Subscription service")
		}
	}
	if safetyPort > 0 {
		add(safetyPort, "tcp", "session", "Current panel connection")
	}
	for _, p := range detectSSHPorts() {
		add(p, "tcp", "ssh", "SSH")
	}
	manual, err := loadManualFirewallRules()
	if err != nil {
		return nil, err
	}
	for _, r := range manual {
		add(r.Port, r.Protocol, "manual", "Manual rule")
	}
	if auto {
		var inbounds []model.Inbound
		err := database.GetDB().Model(&model.Inbound{}).
			Select("port", "protocol", "stream_settings", "settings", "remark", "enable", "node_id").
			Where("enable = ? AND port > 0 AND node_id IS NULL", true).Find(&inbounds).Error
		if err != nil {
			return nil, err
		}
		for _, ib := range inbounds {
			network, _ := inboundStreamHints(string(ib.Protocol), ib.StreamSettings, ib.Settings)
			for _, proto := range firewallProtocolsForInbound(ib.Protocol, network) {
				label := strings.TrimSpace(ib.Remark)
				if label == "" {
					label = string(ib.Protocol)
				}
				add(ib.Port, proto, "inbound", label)
			}
		}
	}
	out := make([]FirewallRule, 0, len(m))
	for _, r := range m {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Port < out[j].Port || (out[i].Port == out[j].Port && out[i].Protocol < out[j].Protocol)
	})
	return out, nil
}

func firewallSourcePriority(s string) int {
	switch s {
	case "session":
		return 60
	case "panel":
		return 50
	case "ssh":
		return 40
	case "subscription":
		return 30
	case "manual":
		return 20
	case "inbound":
		return 10
	default:
		return 0
	}
}

func firewallProtocolsForInbound(protocol model.Protocol, network string) []string {
	switch protocol {
	case model.WireGuard, model.AmneziaWG, model.Hysteria, model.TUIC:
		return []string{"udp"}
	case model.Mixed, model.VKTurnProxy:
		return []string{"tcp", "udp"}
	}
	seen := map[string]bool{}
	for part := range strings.FieldsFuncSeq(strings.ToLower(network), func(r rune) bool { return r == ',' || r == ' ' || r == ';' || r == '/' }) {
		switch strings.TrimSpace(part) {
		case "udp", "kcp", "mkcp", "quic", "hysteria", "wireguard":
			seen["udp"] = true
		case "tcp", "ws", "websocket", "grpc", "http", "httpupgrade", "splithttp", "xhttp":
			seen["tcp"] = true
		}
	}
	if len(seen) == 0 {
		seen["tcp"] = true
	}
	out := []string{}
	if seen["tcp"] {
		out = append(out, "tcp")
	}
	if seen["udp"] {
		out = append(out, "udp")
	}
	return out
}

func normalizeFirewallProtocols(p string) ([]string, error) {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "tcp":
		return []string{"tcp"}, nil
	case "udp":
		return []string{"udp"}, nil
	case "both", "tcp,udp", "udp,tcp":
		return []string{"tcp", "udp"}, nil
	default:
		return nil, errors.New("protocol must be tcp, udp, or both")
	}
}

func firewallRuleKey(port int, proto string) string {
	return strconv.Itoa(port) + "/" + strings.ToLower(proto)
}

func firewallSetting(key, fallback string) (string, error) {
	st, err := (&SettingService{}).getSetting(key)
	if database.IsNotFound(err) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	return st.Value, nil
}

func firewallAutoSync() (bool, error) {
	raw, err := firewallSetting(firewallAutoSyncKey, "true")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(raw) == "" {
		return true, nil
	}
	return strconv.ParseBool(raw)
}

func loadManagedFirewallRules() ([]FirewallRule, error) {
	var out []FirewallRule
	raw, err := firewallSetting(firewallManagedRulesKey, "[]")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func loadManualFirewallRules() ([]FirewallManualRule, error) {
	var out []FirewallManualRule
	raw, err := firewallSetting(firewallManualRulesKey, "[]")
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveFirewallJSON(key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return (&SettingService{}).setString(key, string(b))
}

func detectFirewallBackend(ctx context.Context) (firewallBackend, error) {
	var candidates []firewallBackend
	if path, err := exec.LookPath("ufw"); err == nil {
		b := firewallBackend{name: "ufw", binary: path}
		b.enabled = func(ctx context.Context) (bool, error) {
			out, err := runFirewallCommand(ctx, path, "status")
			return err == nil && strings.Contains(out, "Status: active"), err
		}
		candidates = append(candidates, b)
	}
	if path, err := exec.LookPath("firewall-cmd"); err == nil {
		b := firewallBackend{name: "firewalld", binary: path, zone: "public"}
		if p, err := exec.LookPath("firewall-offline-cmd"); err == nil {
			b.offline = p
		}
		b.enabled = func(ctx context.Context) (bool, error) {
			if systemctl, err := exec.LookPath("systemctl"); err == nil {
				cmd := exec.CommandContext(ctx, systemctl, "is-active", "--quiet", "firewalld")
				cmd.Env = append(os.Environ(), "LC_ALL=C")
				err := cmd.Run()
				if err == nil {
					return true, nil
				}
				if _, ok := err.(*exec.ExitError); ok {
					return false, nil
				}
				return false, err
			}
			out, err := runFirewallCommand(ctx, path, "--state")
			return err == nil && strings.TrimSpace(out) == "running", nil
		}
		if out, err := runFirewallCommand(ctx, path, "--get-default-zone"); err == nil && strings.TrimSpace(out) != "" {
			b.zone = strings.TrimSpace(out)
		} else if b.offline != "" {
			if out, err := runFirewallCommand(ctx, b.offline, "--get-default-zone"); err == nil && strings.TrimSpace(out) != "" {
				b.zone = strings.TrimSpace(out)
			}
		}
		candidates = append(candidates, b)
	}
	for _, b := range candidates {
		if on, _ := b.enabled(ctx); on {
			return b, nil
		}
	}
	if len(candidates) > 0 {
		return candidates[0], nil
	}
	return firewallBackend{}, errors.New("no supported firewall found (install ufw or firewalld)")
}

func listFirewallRules(ctx context.Context, b firewallBackend) (map[string]bool, error) {
	out := map[string]bool{}
	if b.name == "ufw" {
		text, err := runFirewallCommand(ctx, b.binary, "show", "added")
		if err != nil {
			return nil, err
		}
		for line := range strings.SplitSeq(text, "\n") {
			f := strings.Fields(strings.TrimSpace(line))
			if len(f) >= 3 && f[0] == "ufw" && f[1] == "allow" && validFirewallSpec(f[2]) {
				out[strings.ToLower(f[2])] = true
			}
		}
		return out, nil
	}
	on, _ := b.enabled(ctx)
	binary := b.binary
	if !on {
		if b.offline == "" {
			return out, nil
		}
		binary = b.offline
	}
	text, err := runFirewallCommand(ctx, binary, "--zone="+b.zone, "--list-ports")
	if err != nil {
		return nil, err
	}
	for _, spec := range strings.Fields(text) {
		if validFirewallSpec(spec) {
			out[strings.ToLower(spec)] = true
		}
	}
	return out, nil
}

func validFirewallSpec(spec string) bool {
	p := strings.Split(strings.ToLower(spec), "/")
	if len(p) != 2 || (p[1] != "tcp" && p[1] != "udp") {
		return false
	}
	n, err := strconv.Atoi(p[0])
	return err == nil && n > 0 && n <= 65535
}

func addFirewallRule(ctx context.Context, b firewallBackend, r FirewallRule) error {
	spec := firewallRuleKey(r.Port, r.Protocol)
	if b.name == "ufw" {
		_, err := runFirewallCommand(ctx, b.binary, "allow", spec, "comment", "3x-ui managed")
		return err
	}
	on, _ := b.enabled(ctx)
	if !on {
		if b.offline == "" {
			return errors.New("firewalld is stopped and firewall-offline-cmd is unavailable")
		}
		_, err := runFirewallCommand(ctx, b.offline, "--zone="+b.zone, "--add-port="+spec)
		return err
	}
	if _, err := runFirewallCommand(ctx, b.binary, "--permanent", "--zone="+b.zone, "--add-port="+spec); err != nil {
		return err
	}
	_, err := runFirewallCommand(ctx, b.binary, "--zone="+b.zone, "--add-port="+spec)
	return err
}

func deleteFirewallRule(ctx context.Context, b firewallBackend, r FirewallRule) error {
	spec := firewallRuleKey(r.Port, r.Protocol)
	if b.name == "ufw" {
		_, err := runFirewallCommand(ctx, b.binary, "--force", "delete", "allow", spec)
		return err
	}
	on, _ := b.enabled(ctx)
	if !on {
		if b.offline == "" {
			return errors.New("firewalld is stopped and firewall-offline-cmd is unavailable")
		}
		_, err := runFirewallCommand(ctx, b.offline, "--zone="+b.zone, "--remove-port="+spec)
		return err
	}
	if _, err := runFirewallCommand(ctx, b.binary, "--permanent", "--zone="+b.zone, "--remove-port="+spec); err != nil {
		return err
	}
	_, err := runFirewallCommand(ctx, b.binary, "--zone="+b.zone, "--remove-port="+spec)
	return err
}

func setFirewallBackendEnabled(ctx context.Context, b firewallBackend, enabled bool) error {
	if b.name == "ufw" {
		args := []string{"disable"}
		if enabled {
			args = []string{"--force", "enable"}
		}
		_, err := runFirewallCommand(ctx, b.binary, args...)
		return err
	}
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return errors.New("systemctl is required to control firewalld")
	}
	args := []string{"disable", "--now", "firewalld"}
	if enabled {
		args = []string{"enable", "--now", "firewalld"}
	}
	_, err = runFirewallCommand(ctx, systemctl, args...)
	return err
}

func runFirewallCommand(parent context.Context, binary string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(parent, 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(out))
	if ctx.Err() != nil {
		return text, fmt.Errorf("%s timed out", binary)
	}
	if err != nil {
		if text != "" {
			return text, errors.New(text)
		}
		return text, err
	}
	return text, nil
}

func detectSSHPorts() []int {
	ports := map[int]bool{}
	if sshd, err := exec.LookPath("sshd"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		out, e := runFirewallCommand(ctx, sshd, "-T")
		cancel()
		if e == nil {
			for line := range strings.SplitSeq(out, "\n") {
				f := strings.Fields(line)
				if len(f) == 2 && f[0] == "port" {
					if p, e := strconv.Atoi(f[1]); e == nil && p > 0 && p <= 65535 {
						ports[p] = true
					}
				}
			}
		}
	}
	if len(ports) == 0 {
		if raw, err := os.ReadFile("/etc/ssh/sshd_config"); err == nil {
			for line := range strings.SplitSeq(string(raw), "\n") {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				f := strings.Fields(line)
				if len(f) >= 2 && strings.EqualFold(f[0], "Port") {
					if p, e := strconv.Atoi(f[1]); e == nil && p > 0 && p <= 65535 {
						ports[p] = true
					}
				}
			}
		}
	}
	if len(ports) == 0 {
		ports[22] = true
	}
	out := make([]int, 0, len(ports))
	for p := range ports {
		out = append(out, p)
	}
	sort.Ints(out)
	return out
}
