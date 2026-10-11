package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
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
	singBoxProcess        = singbox.NewProcess(singbox.GetConfigPath())
	singBoxTrafficAPI     = singbox.NewConnectionAPIClient()
	singBoxTrafficMu      sync.Mutex
	// A snapshot consumes native API deltas. Keep an unsuccessful DB batch so
	// the next poll can persist it before reading another snapshot.
	singBoxPendingInbound []*coremodel.Traffic
	singBoxPendingClients []*coremodel.ClientTraffic
	singBoxInstallMu      sync.Mutex
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
	Hiddify bool
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
	if s.Hiddify {
		return (&HiddifyConfigGenerator{}).Generate()
	}
	return s.generateNativeConfig(nil)
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
		return nil
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
	rollback := func() {
		_ = singBoxSettingService.SetSingBoxConfigTemplate(old)
		_ = s.WriteConfig()
	}
	if err := s.WriteConfig(); err != nil {
		rollback()
		return err
	}
	if !s.IsRunning() {
		return nil
	}
	if err := s.Validate(ctx); err != nil {
		rollback()
		return err
	}
	if err := s.Restart(ctx); err != nil {
		rollback()
		_ = s.Restart(ctx)
		return err
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
	rollback := func() {
		_ = singBoxSettingService.SetSingBoxConfigTemplate(old)
		_ = s.WriteConfig()
	}
	if err := s.WriteConfig(); err != nil {
		rollback()
		return err
	}
	if !s.IsRunning() {
		return nil
	}
	if err := s.Validate(ctx); err != nil {
		rollback()
		return err
	}
	if err := s.Restart(ctx); err != nil {
		rollback()
		_ = s.Restart(ctx)
		return err
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
	if s.Hiddify && wasRunning {
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
	if s.Hiddify {
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
	if s.Hiddify {
		hiddifyStats.Close()
	}

	tuic.GetManager().StopAll()
	if err := s.restartProcess(ctx); err != nil {
		if s.Hiddify && wasRunning && len(previous) > 0 {
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
	singBoxTrafficMu.Lock()
	singBoxTrafficAPI.Close()
	singBoxTrafficAPI.ResetTrafficBaseline(s.process().GetStartTime().UnixMilli())
	singBoxTrafficMu.Unlock()
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
	if s.Hiddify {
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

	singBoxTrafficMu.Lock()
	singBoxTrafficAPI.Close()
	singBoxTrafficAPI.ResetTrafficBaseline(s.process().GetStartTime().UnixMilli())
	singBoxTrafficMu.Unlock()
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

	if s.Hiddify && s.IsRunning() {
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
	if s.Hiddify {
		sessions, err := s.hiddifySessions(ctx)
		return len(sessions), err
	}
	api := singbox.NewConnectionAPIClient()
	defer api.Close()
	connections, err := api.Snapshot(ctx)
	if err == nil {
		active := 0
		for _, connection := range connections {
			if connection != nil && connection.ClosedAt == 0 {
				active++
			}
		}
		return active, nil
	}
	clashConnections, err := singbox.NewClashStatsClient().Connections(ctx)
	if err != nil {
		return 0, err
	}
	return len(clashConnections), nil
}

// OnlinePresence returns the current sing-box users/IPs and active inbound
// tags from one native API snapshot. Keeping them in the same snapshot makes
// the node's online user and inbound indicators describe the same instant.
func (s *SingBoxService) OnlinePresence(ctx context.Context) (map[string]map[string]struct{}, []string, error) {
	if s.Hiddify {
		return s.hiddifyPresence(ctx)
	}
	api := singbox.NewConnectionAPIClient()
	defer api.Close()
	connections, err := api.Snapshot(ctx)
	if err != nil {
		return nil, nil, err
	}
	online := make(map[string]map[string]struct{})
	activeSet := make(map[string]struct{})
	for _, connection := range connections {
		if connection == nil || connection.ClosedAt > 0 {
			continue
		}
		if connection.Inbound != "" {
			activeSet[connection.Inbound] = struct{}{}
		}
		if connection.User == "" || connection.Source == "" {
			continue
		}
		ip := connection.Source
		if host, _, splitErr := net.SplitHostPort(ip); splitErr == nil {
			ip = host
		}
		if ip == "" {
			continue
		}
		if online[connection.User] == nil {
			online[connection.User] = make(map[string]struct{})
		}
		online[connection.User][ip] = struct{}{}
	}
	active := make([]string, 0, len(activeSet))
	for tag := range activeSet {
		active = append(active, tag)
	}
	sort.Strings(active)
	return online, active, nil
}

func (s *SingBoxService) OnlineClientIPs(ctx context.Context) (map[string]map[string]struct{}, error) {
	online, _, err := s.OnlinePresence(ctx)
	return online, err
}

func (s *SingBoxService) DisconnectClientIPs(ctx context.Context, email string, ips []string) error {
	if email == "" || len(ips) == 0 {
		return nil
	}
	if s.Hiddify {
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
	api := singbox.NewConnectionAPIClient()
	defer api.Close()
	connections, err := api.Snapshot(ctx)
	if err != nil {
		return err
	}
	wanted := make(map[string]struct{}, len(ips))
	for _, ip := range ips {
		if ip != "" {
			wanted[ip] = struct{}{}
		}
	}
	var disconnectErrors []string
	for _, connection := range connections {
		if connection == nil || connection.ClosedAt > 0 || connection.User != email || connection.Source == "" {
			continue
		}
		ip := connection.Source
		if host, _, splitErr := net.SplitHostPort(ip); splitErr == nil {
			ip = host
		}
		if _, ok := wanted[ip]; !ok {
			continue
		}
		if err := api.CloseConnection(ctx, connection.ID); err != nil {
			disconnectErrors = append(disconnectErrors, err.Error())
		}
	}
	if len(disconnectErrors) > 0 {
		return fmt.Errorf("disconnect sing-box sessions: %s", strings.Join(disconnectErrors, "; "))
	}
	return nil
}

func accumulateSingBoxTraffic(up, down *int64, uplinkDelta, downlinkDelta int64) {
	*up = sumTrafficDelta(*up, uplinkDelta)
	*down = sumTrafficDelta(*down, downlinkDelta)
}

func (s *SingBoxService) PollTraffic(ctx context.Context) error {
	if s.Hiddify {
		return s.pollHiddifyTraffic(ctx)
	}
	// The cron callback creates a lightweight SingBoxService value every 5s;
	// keep the native API transport package-wide so traffic polling does not
	// repeatedly allocate a gRPC ClientConn and its background transport state.
	singBoxTrafficMu.Lock()
	defer singBoxTrafficMu.Unlock()
	if len(singBoxPendingInbound) > 0 || len(singBoxPendingClients) > 0 {
		if err := s.commitTraffic(singBoxPendingInbound, singBoxPendingClients); err != nil {
			return err
		}
		singBoxPendingInbound = nil
		singBoxPendingClients = nil
	}
	response, err := singBoxTrafficAPI.SnapshotTrafficEvents(ctx)
	if err != nil {
		return err
	}

	inboundDeltas := make(map[string]*coremodel.Traffic)
	clientDeltas := make(map[string]*coremodel.ClientTraffic)
	onlineEmails := make(map[string]struct{})

	for _, event := range response.Events {
		if event == nil || event.Connection == nil {
			continue
		}
		connection := event.Connection
		if event.Type != singbox.ConnectionEventClosed && connection.User != "" {
			onlineEmails[connection.User] = struct{}{}
		}

		uplinkDelta := event.UplinkDelta
		downlinkDelta := event.DownlinkDelta
		if uplinkDelta == 0 && downlinkDelta == 0 {
			continue
		}

		if connection.Inbound != "" {
			traffic := inboundDeltas[connection.Inbound]
			if traffic == nil {
				traffic = &coremodel.Traffic{Tag: connection.Inbound, IsInbound: true}
				inboundDeltas[connection.Inbound] = traffic
			}
			accumulateSingBoxTraffic(&traffic.Up, &traffic.Down, uplinkDelta, downlinkDelta)
		}
		if connection.User != "" {
			traffic := clientDeltas[connection.User]
			if traffic == nil {
				traffic = &coremodel.ClientTraffic{Email: connection.User}
				clientDeltas[connection.User] = traffic
			}
			accumulateSingBoxTraffic(&traffic.Up, &traffic.Down, uplinkDelta, downlinkDelta)
		}
	}

	inboundTraffic := make([]*coremodel.Traffic, 0, len(inboundDeltas))
	for _, traffic := range inboundDeltas {
		inboundTraffic = append(inboundTraffic, traffic)
	}
	clientTraffic := make([]*coremodel.ClientTraffic, 0, len(clientDeltas))
	for _, traffic := range clientDeltas {
		clientTraffic = append(clientTraffic, traffic)
	}
	{
		if err = s.commitTraffic(inboundTraffic, clientTraffic); err != nil {
			singBoxPendingInbound = inboundTraffic
			singBoxPendingClients = clientTraffic
			return err
		}
	}

	emails := make([]string, 0, len(onlineEmails))
	for email := range onlineEmails {
		emails = append(emails, email)
	}
	sort.Strings(emails)
	if len(emails) > 0 {
		return singBoxInboundService.BumpClientsLastOnline(emails)
	}
	return nil
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

func (s *SingBoxService) installVersion(ctx context.Context, installer func(context.Context) (string, error)) (string, error) {
	singBoxInstallMu.Lock()
	defer singBoxInstallMu.Unlock()
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()

	restore, cleanup, err := singbox.SnapshotInstallation()
	if err != nil {
		return "", fmt.Errorf("backup sing-box installation: %w", err)
	}
	keepBackup := false
	defer func() {
		if !keepBackup {
			cleanup()
		}
	}()
	wasRunning := s.IsRunning()
	if wasRunning {
		if err := s.stopLocked(ctx); err != nil {
			return "", fmt.Errorf("stop sing-box before update: %w", err)
		}
	}
	rollback := func(cause error) (string, error) {
		// The update request may have been cancelled; recovery must still work.
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := s.stopProcess(ctx); err != nil {
			keepBackup = true
			return "", fmt.Errorf("%w; stop failed update: %w", cause, err)
		}
		if err := restore(); err != nil {
			keepBackup = true
			return "", fmt.Errorf("%w; restore previous installation: %w", cause, err)
		}
		if wasRunning {
			if _, err := s.process().Version(recoveryCtx); err != nil {
				return "", fmt.Errorf("%w; read restored version: %w", cause, err)
			}
			// Run the restored applied config, not a freshly generated candidate.
			if err := s.process().Start(recoveryCtx); err != nil {
				return "", fmt.Errorf("%w; restart restored installation: %w", cause, err)
			}
			s.stoppedFlag().Store(false)
		}
		s.process().SetError(cause)
		return "", cause
	}
	installed, err := installer(ctx)
	if err != nil {
		return rollback(fmt.Errorf("install sing-box: %w", err))
	}
	currentVersion, err := s.Version(ctx)
	if err != nil {
		return rollback(fmt.Errorf("verify installed sing-box %q: %w", installed, err))
	}
	if wasRunning {
		if err := s.startLocked(ctx); err != nil {
			return rollback(fmt.Errorf("start updated sing-box %s: %w", currentVersion, err))
		}
	}
	return currentVersion, nil
}

func (s *SingBoxService) InstallLatest(ctx context.Context) (string, error) {
	if s.Hiddify {
		return "", fmt.Errorf("hiddify-core is updated with the pinned panel release")
	}
	return s.installVersion(ctx, singbox.InstallLatest)
}

func (s *SingBoxService) ListVersions(ctx context.Context) ([]singbox.ReleaseVersion, error) {
	return singbox.ListVersions(ctx)
}

func (s *SingBoxService) InstallVersion(ctx context.Context, version string) (string, error) {
	if s.Hiddify {
		return "", fmt.Errorf("hiddify-core is updated with the pinned panel release")
	}
	return s.installVersion(ctx, func(ctx context.Context) (string, error) {
		return singbox.InstallVersion(ctx, version)
	})
}

func (s *SingBoxService) Uninstall(ctx context.Context) error {
	if s.Hiddify {
		return fmt.Errorf("hiddify-core belongs to the panel release")
	}
	if s.IsRunning() {
		return fmt.Errorf("stop sing-box before uninstalling it")
	}
	return singbox.Uninstall()
}

func (s *SingBoxService) BinaryPath() string {
	if s.Hiddify {
		return hiddify.GetBinaryPath()
	}
	return singbox.GetBinaryPath()
}

func (s *SingBoxService) ProcessConfigPath() string {
	if s.Hiddify {
		return hiddify.GetConfigPath()
	}
	return singbox.GetConfigPath()
}

func (s *SingBoxService) commitTraffic(inbounds []*coremodel.Traffic, clients []*coremodel.ClientTraffic) error {
	needRestart, disabled, err := singBoxInboundService.AddTraffic(inbounds, clients)
	if needRestart || disabled {
		s.SetToNeedRestart()
	}
	return err
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
