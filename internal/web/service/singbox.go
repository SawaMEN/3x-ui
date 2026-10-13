package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/database/model"
	"github.com/SawaMEN/3x-ui/v3/internal/externalvpn"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/tuic"
	"github.com/SawaMEN/3x-ui/v3/internal/util/tail"
)

var (
	singBoxInboundService InboundService
	singBoxSettingService SettingService
	singBoxApplyMu        sync.Mutex
	singBoxTemplateMu     sync.Mutex
)

// SetSingBoxDependencies wires the panel services used by the sing-box
// config generator. It mirrors the existing Xray service lifecycle wiring.
func SetSingBoxDependencies(inbound *InboundService, settings *SettingService) {
	if inbound != nil {
		singBoxInboundService = *inbound
	}
	if settings != nil {
		singBoxSettingService = *settings
	}
}

type SingBoxService struct {
	gateway *GatewayNetworkService
}

func singBoxInboundRequiresUsers(protocol model.Protocol) bool {
	switch protocol {
	case model.VLESS, model.VMESS, model.Shadowsocks, model.Trojan, model.MASQUE, model.Snell, model.NaiveProxy, model.Hysteria, model.ShadowTLS, model.AnyTLS, model.TUIC:
		return true
	default:
		return false
	}
}

// These listeners run in their own local processes, outside sing-box.
// Never emit them as sing-box inbounds or reserve sing-box client lookups for them.
func isLocalSidecarInbound(protocol model.Protocol) bool {
	switch protocol {
	case model.MTProto, model.AmneziaWG, model.Mieru,
		model.Pingtunnel, model.TrustTunnel, model.FPTN, model.OpenFlux, model.Sudoku, model.VKTurnProxy:
		return true
	default:
		return false
	}
}

func mustJSON(value map[string]any) []byte {
	data, _ := json.Marshal(value)
	return data
}

// singBoxTUICInbound uses the same credentials and TLS certificate as the
// external TUIC server. The native listener exposes authenticated user names
// to the sing-box traffic API, so client quotas receive real byte deltas.
func singBoxTUICInbound(ib *model.Inbound, clients []any) (map[string]any, error) {
	inst, ok := tuic.InstanceFromInbound(ib)
	if !ok {
		return nil, fmt.Errorf("invalid TUIC inbound %q settings", ib.Tag)
	}
	if strings.TrimSpace(inst.Certificate) == "" || strings.TrimSpace(inst.PrivateKey) == "" {
		return nil, fmt.Errorf("TUIC inbound %q requires a TLS certificate and private key", ib.Tag)
	}
	users := make([]map[string]any, 0, len(clients))
	for _, item := range clients {
		client, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := client["email"].(string)
		uuid, _ := client["uuid"].(string)
		password, _ := client["password"].(string)
		if name == "" || uuid == "" || password == "" {
			return nil, fmt.Errorf("TUIC inbound %q has an active client without email, UUID or password", ib.Tag)
		}
		users = append(users, map[string]any{"name": name, "uuid": uuid, "password": password})
	}
	listen := strings.TrimSpace(inst.Listen)
	if listen == "" {
		listen = "0.0.0.0"
	}
	if inst.Port < 1 || inst.Port > 65535 {
		return nil, fmt.Errorf("TUIC inbound %q has an invalid port", ib.Tag)
	}
	tls := map[string]any{"enabled": true, "alpn": inst.ALPN}
	if strings.Contains(inst.Certificate, "-----BEGIN CERTIFICATE-----") {
		tls["certificate"] = strings.Split(inst.Certificate, "\n")
		tls["key"] = strings.Split(inst.PrivateKey, "\n")
	} else {
		tls["certificate_path"], tls["key_path"] = inst.Certificate, inst.PrivateKey
	}
	return map[string]any{
		"type": "tuic", "tag": ib.Tag, "listen": listen, "listen_port": inst.Port,
		"users": users, "congestion_control": inst.CongestionControl,
		"auth_timeout":       fmt.Sprintf("%ds", inst.AuthenticationTimeout),
		"zero_rtt_handshake": inst.ZeroRTTHandshake,
		"tls":                tls,
	}, nil
}

// GetConfig is the single generation entry point for previews, candidates and restarts.
func (s *SingBoxService) GetConfig() (*singbox.Config, error) {
	return (&HiddifyConfigGenerator{}).Generate()
}

type SingBoxEditorSnapshot struct {
	InboundTags     []string       `json:"inboundTags"`
	Config          map[string]any `json:"config"`
	Running         bool           `json:"running"`
	Version         string         `json:"version"`
	ConfigPath      string         `json:"configPath"`
	ConfigSource    string         `json:"configSource"`
	ConfigModified  string         `json:"configModified"`
	ManagedSections []string       `json:"managedSections"`
}

func (s *SingBoxService) GetEditorConfig(ctx context.Context) (*SingBoxEditorSnapshot, error) {
	path := s.ProcessConfigPath()
	data, err := os.ReadFile(path)
	source := "disk"
	if os.IsNotExist(err) {
		cfg, cfgErr := s.GetConfig()
		if cfgErr == nil {
			data, err = cfg.Marshal()
		} else {
			rawTemplate, templateErr := singBoxSettingService.GetSingBoxConfigTemplate()
			if templateErr != nil {
				return nil, cfgErr
			}
			rawTemplate = strings.TrimSpace(rawTemplate)
			if rawTemplate == "" {
				data, err = singbox.NewConfig().Marshal()
			} else {
				data = []byte(rawTemplate)
				err = nil
			}
		}
		source = "generated"
	}
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	// Remove the named panel-managed filter from an editor round trip.
	var editorConfig singbox.Config
	if err := json.Unmarshal(data, &editorConfig); err != nil {
		return nil, err
	}
	if err := stripSingBoxAdBlock(&editorConfig); err != nil {
		return nil, err
	}
	if route, ok := raw["route"]; ok && route != nil {
		raw["route"] = editorConfig.Route
	}
	if _, ok := raw["outbounds"]; ok {
		raw["outbounds"] = editorConfig.Outbounds
	}
	inboundTags := make([]string, 0, len(editorConfig.Inbounds))
	for _, inbound := range editorConfig.Inbounds {
		if tag, ok := inbound["tag"].(string); ok && tag != "" {
			inboundTags = append(inboundTags, tag)
		}
	}
	managedRows, err := singBoxInboundService.GetAllInbounds()
	if err != nil {
		return nil, err
	}
	managedTags := map[string]bool{}
	for _, row := range managedRows {
		if row != nil && row.NodeID == nil && row.Protocol == model.MASQUE {
			managedTags[row.Tag] = true
		}
	}
	editableEndpoints := make([]map[string]any, 0, len(editorConfig.Endpoints))
	for _, endpoint := range editorConfig.Endpoints {
		tag, _ := endpoint["tag"].(string)
		if managedTags[tag] {
			inboundTags = append(inboundTags, tag)
		} else {
			editableEndpoints = append(editableEndpoints, endpoint)
		}
	}
	if _, exists := raw["endpoints"]; exists {
		raw["endpoints"] = editableEndpoints
	}
	delete(raw, "inbounds")
	delete(raw, "services")
	modified := ""
	if info, statErr := os.Stat(path); statErr == nil {
		modified = info.ModTime().UTC().Format(time.RFC3339)
	}
	version := "Unknown"
	if v, versionErr := s.CachedVersion(ctx); versionErr == nil && strings.TrimSpace(v) != "" {
		version = v
	}
	return &SingBoxEditorSnapshot{
		Config:          raw,
		InboundTags:     inboundTags,
		Running:         s.IsRunning(),
		Version:         version,
		ConfigPath:      path,
		ConfigSource:    source,
		ConfigModified:  modified,
		ManagedSections: []string{"inbounds", "services"},
	}, nil
}

func normalizeSingBoxTemplate(raw string) (string, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &obj); err != nil {
		return "", fmt.Errorf("invalid sing-box JSON: %w", err)
	}
	if obj == nil {
		return "", fmt.Errorf("sing-box settings must be a JSON object")
	}

	for key := range obj {
		if key == "inbounds" || key == "services" {
			delete(obj, key)
			continue
		}
		if !nativeTemplateFields[key] {
			return "", fmt.Errorf("unsupported sing-box top-level field %q; panel-managed inbounds/services are not editable here", key)
		}
	}
	var cfg singbox.Config
	if data, ok := obj["outbounds"]; ok {
		if err := json.Unmarshal(data, &cfg.Outbounds); err != nil {
			return "", fmt.Errorf("invalid native outbounds: %w", err)
		}
	}
	if data, ok := obj["experimental"]; ok {
		if err := json.Unmarshal(data, &cfg.Experimental); err != nil {
			return "", fmt.Errorf("invalid native experimental settings: %w", err)
		}
	}
	if err := ensureWARPMASQUECache(&cfg); err != nil {
		return "", err
	}
	if cfg.Experimental != nil {
		data, err := json.Marshal(cfg.Experimental)
		if err != nil {
			return "", err
		}
		obj["experimental"] = data
	}
	data, err := json.MarshalIndent(obj, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// mergeManagedMASQUEEndpoints preserves manually configured endpoints while
// panel-owned tags always use the current database state (including absence).
func mergeManagedMASQUEEndpoints(template, generated []map[string]any, tags map[string]bool) []map[string]any {
	endpoints := make([]map[string]any, 0)
	for _, endpoint := range template {
		tag, _ := endpoint["tag"].(string)
		if !tags[tag] {
			endpoints = append(endpoints, endpoint)
		}
	}
	return append(endpoints, generated...)
}

func (s *SingBoxService) applyNativeTemplate(cfg *singbox.Config, managedMASQUETags map[string]bool) error {
	rawTemplate, err := singBoxSettingService.GetSingBoxConfigTemplate()
	if err != nil {
		return err
	}
	return s.applyNativeTemplateValue(cfg, managedMASQUETags, rawTemplate)
}

func (s *SingBoxService) applyNativeTemplateValue(cfg *singbox.Config, managedMASQUETags map[string]bool, rawTemplate string) error {
	rawTemplate = strings.TrimSpace(rawTemplate)
	if rawTemplate == "" {
		return ensureWARPMASQUECache(cfg)
	}
	var patch map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rawTemplate), &patch); err != nil {
		return fmt.Errorf("stored sing-box settings are invalid: %w", err)
	}
	if patch == nil {
		return fmt.Errorf("stored %s settings must be an object", s.coreType())
	}
	for key := range patch {
		if !nativeTemplateFields[key] {
			return fmt.Errorf("unsupported %s top-level field %q", s.coreType(), key)
		}
	}
	// The native editor may replace endpoints, but panel-managed MASQUE
	// listeners must continue to follow inbound/client state from the database.
	managedMASQUE := make([]map[string]any, 0)
	for _, endpoint := range cfg.Endpoints {
		if endpoint["type"] == "masque-server" {
			managedMASQUE = append(managedMASQUE, endpoint)
		}
	}
	currentData, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	var merged map[string]json.RawMessage
	if err := json.Unmarshal(currentData, &merged); err != nil {
		return err
	}
	for key, value := range patch {
		if key == "inbounds" || key == "services" {
			continue
		}
		if key == "route" {
			if route, ok := mergeGatewayOnlyRoute(merged[key], value); ok {
				merged[key] = route
				continue
			}
		}
		merged[key] = value
	}
	mergedData, err := json.Marshal(merged)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(mergedData, cfg); err != nil {
		return err
	}
	if err := mergeSingBoxGatewayInbound(cfg, patch["inbounds"]); err != nil {
		return err
	}
	// Include disabled listeners and listeners without active users in the tag
	// set, so an old template cannot restore their previous credentials.
	cfg.Endpoints = mergeManagedMASQUEEndpoints(cfg.Endpoints, managedMASQUE, managedMASQUETags)
	if err := ensureWARPMASQUECache(cfg); err != nil {
		return err
	}

	var referencesLocal func(any) bool
	referencesLocal = func(value any) bool {
		switch value := value.(type) {
		case map[string]any:
			for key, item := range value {
				if key == "domain_resolver" || key == "default_domain_resolver" {
					if name, ok := item.(string); ok && name == "local" {
						return true
					}
					if resolver, ok := item.(map[string]any); ok && resolver["server"] == "local" {
						return true
					}
					continue
				}
				if referencesLocal(item) {
					return true
				}
			}
		case []any:
			for _, item := range value {
				if referencesLocal(item) {
					return true
				}
			}
		case []map[string]any:
			for _, item := range value {
				if referencesLocal(item) {
					return true
				}
			}
		}
		return false
	}
	if referencesLocal(cfg.Inbounds) || referencesLocal(cfg.Route) || referencesLocal(cfg.Outbounds) || referencesLocal(cfg.Endpoints) {
		if cfg.DNS == nil {
			cfg.DNS = map[string]any{}
		}
		data, _ := json.Marshal(cfg.DNS["servers"])
		var servers []map[string]any
		if err := json.Unmarshal(data, &servers); err != nil && string(data) != "null" {
			return fmt.Errorf("invalid native DNS servers: %w", err)
		}
		hasLocal := false
		for _, server := range servers {
			if server["tag"] == "local" {
				hasLocal = true
			}
		}
		if !hasLocal {
			cfg.DNS["servers"] = append(servers, map[string]any{"type": "local", "tag": "local"})
		}
	}
	return nil
}

func (s *SingBoxService) SaveTemplate(ctx context.Context, raw string) error {
	singBoxTemplateMu.Lock()
	defer singBoxTemplateMu.Unlock()
	normalized, err := normalizeSingBoxTemplate(raw)
	if err != nil {
		return err
	}
	old, err := singBoxSettingService.GetSingBoxConfigTemplate()
	if err != nil {
		return err
	}
	if err := singBoxSettingService.SetSingBoxConfigTemplate(normalized); err != nil {
		return err
	}
	rollback := func() error {
		return errors.Join(singBoxSettingService.SetSingBoxConfigTemplate(old), s.WriteConfig())
	}
	if err := s.WriteConfig(); err != nil {
		return errors.Join(err, rollback())
	}
	if !s.IsRunning() {
		return nil
	}
	if err := s.Restart(ctx); err != nil {
		return errors.Join(err, rollback(), s.Restart(ctx))
	}
	return nil
}

func (s *SingBoxService) ResetTemplate(ctx context.Context) error {
	singBoxTemplateMu.Lock()
	defer singBoxTemplateMu.Unlock()
	old, err := singBoxSettingService.GetSingBoxConfigTemplate()
	if err != nil {
		return err
	}
	if err := singBoxSettingService.SetSingBoxConfigTemplate(""); err != nil {
		return err
	}
	rollback := func() error {
		return errors.Join(singBoxSettingService.SetSingBoxConfigTemplate(old), s.WriteConfig())
	}
	if err := s.WriteConfig(); err != nil {
		return errors.Join(err, rollback())
	}
	if !s.IsRunning() {
		return nil
	}
	if err := s.Restart(ctx); err != nil {
		return errors.Join(err, rollback(), s.Restart(ctx))
	}
	return nil
}

func (s *SingBoxService) WriteConfig() error {
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	return s.writeConfigCandidate(context.Background())
}

func (s *SingBoxService) Restart(ctx context.Context) error {
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	return s.restartLocked(ctx)
}

func (s *SingBoxService) restartLocked(ctx context.Context) (result error) {
	unchanged, err := nativeConfigBatchGuard()
	if err != nil {
		return err
	}
	wasRunning := s.IsRunning()
	if !wasRunning {
		if err := s.guardStoppedGateway(ctx); err != nil {
			return err
		}
	}
	previous := s.process().AppliedConfig()
	if _, err := s.process().Version(ctx); err != nil {
		s.process().SetError(err)
		return err
	}
	candidate, err := s.prepareConfigCandidate(ctx)
	if err != nil {
		s.process().SetError(err)
		return err
	}
	defer candidate.close()
	if !unchanged() {
		return errNativeConfigBatchChanged
	}
	if wasRunning {
		if err := s.PollTraffic(ctx); err != nil {
			return err
		}
	}
	// Traffic enforcement can itself commit client changes. Regenerate on the
	// next tick instead of starting a candidate with newly disabled identities.
	if !unchanged() {
		return errNativeConfigBatchChanged
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var finishSidecars func(bool) error
	applied := false
	{
		finishSidecars, err = externalvpn.ApplyOutbounds(candidate.managedOutbounds)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, finishSidecars(applied)) }()
	}
	if !unchanged() {
		return errNativeConfigBatchChanged
	}
	if err := candidate.publish(ctx); err != nil {
		s.process().SetError(err)
		return err
	}
	finishGateway, err := s.pauseGateway(ctx)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, finishGateway()) }()
	{
		hiddifyStats.Close()
	}

	tuic.GetManager().StopAll()
	if err := s.restartProcess(ctx); err != nil {
		if wasRunning && len(previous) > 0 {
			recoveryErr := writeNativeConfigBytes(s.ProcessConfigPath(), previous)
			if recoveryErr == nil && !s.IsRunning() {
				recoveryCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				recoveryErr = s.startProcess(recoveryCtx)
				cancel()
			}
			if recoveryErr != nil {
				err = errors.Join(err, fmt.Errorf("restore previous hiddify configuration: %w", recoveryErr))
			}
			s.process().SetError(err)
		}
		return err
	}
	s.stoppedFlag().Store(false)
	commitManagedYouTube(s.coreType())
	applied = true
	return nil
}

func (s *SingBoxService) Start(ctx context.Context) error {
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	return s.startLocked(ctx)
}

func (s *SingBoxService) startLocked(ctx context.Context) (result error) {
	if s.IsRunning() {
		return s.resumeGateway(ctx)
	}
	if err := s.guardStoppedGateway(ctx); err != nil {
		return err
	}
	if _, err := s.process().Version(ctx); err != nil {
		s.process().SetError(err)
		return err
	}
	unchanged, err := nativeConfigBatchGuard()
	if err != nil {
		return err
	}
	candidate, err := s.prepareConfigCandidate(ctx)
	if err != nil {
		s.process().SetError(err)
		return err
	}
	defer candidate.close()
	if !unchanged() {
		return errNativeConfigBatchChanged
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	applied := false
	{
		finish, err := externalvpn.ApplyOutbounds(candidate.managedOutbounds)
		if err != nil {
			return err
		}
		defer func() { result = errors.Join(result, finish(applied)) }()
	}
	if !unchanged() {
		return errNativeConfigBatchChanged
	}
	if err := candidate.publish(ctx); err != nil {
		s.process().SetError(err)
		return err
	}

	tuic.GetManager().StopAll()
	finishGateway, err := s.pauseGateway(ctx)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, finishGateway()) }()
	if err := s.startProcess(ctx); err != nil {
		return err
	}

	s.stoppedFlag().Store(false)
	commitManagedYouTube(s.coreType())
	applied = true
	return nil
}

func (s *SingBoxService) Stop(ctx context.Context) error {
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	return s.stopLocked(ctx)
}

func (s *SingBoxService) stopLocked(ctx context.Context) (result error) {
	// Shutdown callers may have already cancelled the panel context.
	recovery, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	finish, err := s.pauseGateway(recovery)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, finish()) }()

	if s.IsRunning() {
		if err := s.PollTraffic(ctx); err != nil {
			logger.Warning("flush hiddify traffic before stop:", err)
		}
		hiddifyStats.Close()
	}
	if err := s.stopProcess(ctx); err != nil {
		return err
	}
	s.stoppedFlag().Store(true)
	return nil
}

func (s *SingBoxService) IsRunning() bool {
	return s.process().IsRunning()
}

func (s *SingBoxService) LastError() error {
	return s.process().GetErr()
}

func (s *SingBoxService) Version(ctx context.Context) (string, error) {
	return s.process().Version(ctx)
}

func (s *SingBoxService) CachedVersion(ctx context.Context) (string, error) {
	if version := s.process().GetVersion(); version != "" && version != "Unknown" {
		return version, nil
	}
	return s.process().Version(ctx)
}

func (s *SingBoxService) Validate(ctx context.Context) error {
	return s.process().Validate(ctx)
}

func (s *SingBoxService) ConnectionCount(ctx context.Context) (int, error) {
	{
		sessions, err := s.hiddifySessions(ctx)
		return len(sessions), err
	}
}

// OnlinePresence returns the current sing-box users/IPs and active inbound
// tags from one native API snapshot. Keeping them in the same snapshot makes
// the node's online user and inbound indicators describe the same instant.
func (s *SingBoxService) OnlinePresence(ctx context.Context) (map[string]map[string]struct{}, []string, error) {
	{
		return s.hiddifyPresence(ctx)
	}
}

func (s *SingBoxService) OnlineClientIPs(ctx context.Context) (map[string]map[string]struct{}, error) {
	online, _, err := s.OnlinePresence(ctx)
	return online, err
}

func (s *SingBoxService) DisconnectClientIPs(ctx context.Context, email string, ips []string) error {
	if email == "" || len(ips) == 0 {
		return nil
	}
	{
		_, err := s.disconnectHiddify(ctx, func(session singbox.ActiveSession) bool {
			host, _, _ := net.SplitHostPort(session.Source)
			for _, ip := range ips {
				if normalized := hiddify.NormalizeIP(ip); normalized != "" && host == normalized && session.User == email {
					return true
				}
			}
			return false
		})
		return err
	}
}

func (s *SingBoxService) PollTraffic(ctx context.Context) error {
	{
		return s.pollHiddifyTraffic(ctx)
	}
	// The cron callback creates a lightweight SingBoxService value every 5s;
	// keep the native API transport package-wide so traffic polling does not
	// repeatedly allocate a gRPC ClientConn and its background transport state.
}

// GetLogs returns sing-box process log lines in the same shape used by the
// existing core log viewer. sing-box writes its structured runtime log to the
// panel-managed log file configured in Config.Log.output.
func (s *SingBoxService) GetLogs(count string, filter string) []LogEntry {
	limit, err := strconv.Atoi(count)
	if err != nil || limit <= 0 {
		limit = 100
	}
	lines, err := tail.ReadTailLines(s.logPath(), 0, tail.DefaultTailBytes)
	if err != nil {
		return []LogEntry{}
	}
	filter = strings.TrimSpace(filter)
	filterLower := strings.ToLower(filter)
	entries := make([]LogEntry, 0, min(limit, len(lines)))
	for _, rawLine := range lines {
		if len(entries) >= limit {
			break
		}
		line := strings.TrimSpace(rawLine)
		if line == "" || (filterLower != "" && !strings.Contains(strings.ToLower(line), filterLower)) {
			continue
		}
		entries = append(entries, LogEntry{
			DateTime:  time.Now(),
			Inbound:   "sing-box",
			Outbound:  "sing-box",
			ToAddress: line,
		})
	}
	return entries
}

func (s *SingBoxService) BinaryPath() string {
	return hiddify.GetBinaryPath()
}

func (s *SingBoxService) ProcessConfigPath() string {
	return hiddify.GetConfigPath()
}

func init() {
	singbox.SetRuntimeConfigReader(func() []byte { return SelectedNativeCore().process().AppliedConfig() })
}

var nativeTemplateFields = map[string]bool{
	"$schema": true, "log": true, "dns": true, "ntp": true, "certificate": true,
	"certificate_providers": true, "http_clients": true, "network_namespaces": true,
	"outbounds": true, "route": true, "endpoints": true, "experimental": true,
	"inbounds": true, "services": true,
}
