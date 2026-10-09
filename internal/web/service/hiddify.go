package service

import (
	"context"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

var hiddifyProcess = hiddify.NewProcess(hiddify.GetConfigPath(), false)
var hiddifyNeedRestart atomic.Bool
var hiddifyManuallyStopped atomic.Bool
var hiddifyStats singbox.V2RayStatsClient
var hiddifyPendingInbounds []*xray.Traffic
var hiddifyPendingClients []*xray.ClientTraffic

func IsNativeCore(core string) bool { return core == CoreTypeSingBox || core == CoreTypeHiddify }
func NativeCore(core string) *SingBoxService {
	return &SingBoxService{Hiddify: core == CoreTypeHiddify}
}
func SelectedNativeCore() *SingBoxService {
	core, _ := (&SettingService{}).GetCoreType()
	return NativeCore(core)
}
func (s *SingBoxService) CoreType() string { return s.coreType() }
func (s *SingBoxService) coreType() string {
	if s.Hiddify {
		return CoreTypeHiddify
	}
	return CoreTypeSingBox
}
func (s *SingBoxService) process() *singbox.Process {
	if s.Hiddify {
		return hiddifyProcess
	}
	return singBoxProcess
}
func (s *SingBoxService) candidate(path string) *singbox.Process {
	if s.Hiddify {
		return hiddify.NewProcess(path, true)
	}
	return singbox.NewTestProcess(path)
}
func (s *SingBoxService) translateInbound(raw map[string]any) (map[string]any, error) {
	if s.Hiddify {
		return hiddify.TranslateInbound(raw)
	}
	return singbox.TranslateXrayInbound(raw)
}
func (s *SingBoxService) translateOutbound(raw map[string]any) (map[string]any, error) {
	if s.Hiddify {
		return hiddify.TranslateOutbound(raw)
	}
	return singbox.TranslateXrayOutbound(raw)
}
func (s *SingBoxService) restartFlag() *atomic.Bool {
	if s.Hiddify {
		return &hiddifyNeedRestart
	}
	return &isNeedSingBoxRestart
}
func (s *SingBoxService) stoppedFlag() *atomic.Bool {
	if s.Hiddify {
		return &hiddifyManuallyStopped
	}
	return &isSingBoxManuallyStopped
}
func (s *SingBoxService) logPath() string {
	return filepath.Join(filepath.Dir(s.ProcessConfigPath()), s.coreType()+".log")
}

// QueryStats resets one snapshot only after the previous DB batch was saved.
// Retain a failed batch so a transient DB error cannot discard byte deltas.
func (s *SingBoxService) pollHiddifyTraffic(ctx context.Context) error {
	singBoxTrafficMu.Lock()
	defer singBoxTrafficMu.Unlock()
	if len(hiddifyPendingInbounds) > 0 || len(hiddifyPendingClients) > 0 {
		if err := s.commitTraffic(hiddifyPendingInbounds, hiddifyPendingClients); err != nil {
			return err
		}
		hiddifyPendingInbounds, hiddifyPendingClients = nil, nil
	}
	stats, err := hiddifyStats.Query(ctx, nil, true)
	if err != nil {
		return fmt.Errorf("hiddify user statistics: %w", err)
	}
	hiddifyPendingInbounds, hiddifyPendingClients = splitHiddifyStats(stats)
	if err := s.commitTraffic(hiddifyPendingInbounds, hiddifyPendingClients); err != nil {
		return err
	}
	hiddifyPendingInbounds, hiddifyPendingClients = nil, nil
	return nil
}

func configureHiddifyStats(cfg *singbox.Config) {
	cfg.Hiddify = true
	cfg.Services = nil // v5 uses V2Ray stats and Clash, not sing-box 1.14 service.api.
	users, tags := []string{}, []string{}
	listeners := append(append([]map[string]any{}, cfg.Inbounds...), cfg.Endpoints...)
	for _, inbound := range listeners {
		if tag, ok := inbound["tag"].(string); ok {
			tags = append(tags, tag)
		}
		data, _ := inbound["users"].([]map[string]any)
		for _, user := range data {
			name, _ := user["name"].(string)
			if name == "" {
				name, _ = user["username"].(string)
			}
			if name != "" {
				users = append(users, name)
			}
		}
		anyUsers, _ := inbound["users"].([]any)
		for _, item := range anyUsers {
			if user, ok := item.(map[string]any); ok {
				name, _ := user["name"].(string)
				if name == "" {
					name, _ = user["username"].(string)
				}
				if name != "" {
					users = append(users, name)
				}
			}
		}
	}

	if cfg.Experimental == nil {
		cfg.Experimental = map[string]any{}
	}
	cfg.Experimental["v2ray_api"] = map[string]any{"listen": "127.0.0.1:10086", "stats": map[string]any{"enabled": true, "inbounds": tags, "users": users}}
}

func (s *SingBoxService) hiddifySessions(ctx context.Context) ([]singbox.ActiveSession, error) {
	connections, err := singbox.NewClashStatsClient().Connections(ctx)
	if err != nil {
		return nil, err
	}
	sessions := make([]singbox.ActiveSession, 0, len(connections))
	for _, connection := range connections {
		kind, inbound, _ := strings.Cut(connection.Metadata.Type, "/")
		sessions = append(sessions, singbox.ActiveSession{
			ID: connection.ID, Core: CoreTypeHiddify, Inbound: inbound, InboundType: kind,
			User: connection.Metadata.User, Network: connection.Metadata.Network,
			Source:      net.JoinHostPort(connection.Metadata.SourceIP, connection.Metadata.SourcePort),
			Destination: net.JoinHostPort(connection.Metadata.DestinationIP, connection.Metadata.DestinationPort),
			Domain:      connection.Metadata.Host, CreatedAt: connection.Start.UnixMilli(),
			Upload: connection.Upload, Download: connection.Download, Chain: connection.Chains,
		})
	}
	return sessions, nil
}
func (s *SingBoxService) hiddifyPresence(ctx context.Context) (map[string]map[string]struct{}, []string, error) {
	sessions, err := s.hiddifySessions(ctx)
	if err != nil {
		return nil, nil, err
	}
	online := map[string]map[string]struct{}{}
	tags := map[string]bool{}
	for _, session := range sessions {
		if session.Inbound != "" {
			tags[session.Inbound] = true
		}
		if session.User == "" {
			continue
		}
		host, _, err := net.SplitHostPort(session.Source)
		if err != nil || host == "" {
			continue
		}
		if online[session.User] == nil {
			online[session.User] = map[string]struct{}{}
		}
		online[session.User][host] = struct{}{}
	}
	active := []string{}
	for tag := range tags {
		active = append(active, tag)
	}
	sort.Strings(active)
	return online, active, nil
}
func (s *SingBoxService) disconnectHiddify(ctx context.Context, match func(singbox.ActiveSession) bool) (int, error) {
	sessions, err := s.hiddifySessions(ctx)
	if err != nil {
		return 0, err
	}
	api := singbox.NewClashStatsClient()
	count := 0
	var failures []error
	for _, session := range sessions {
		if !match(session) {
			continue
		}
		if err := api.CloseConnection(ctx, session.ID); err != nil {
			failures = append(failures, err)
		} else {
			count++
		}
	}
	return count, errors.Join(failures...)
}

func splitHiddifyStats(stats []singbox.V2RayStat) (inboundTraffic []*xray.Traffic, clientTraffic []*xray.ClientTraffic) {
	inbounds := map[string]*xray.Traffic{}
	clients := map[string]*xray.ClientTraffic{}
	for _, stat := range stats {
		parts := strings.Split(stat.Name, ">>>")
		if len(parts) != 4 || parts[2] != "traffic" || (parts[3] != "uplink" && parts[3] != "downlink") || stat.Value <= 0 {
			continue
		}
		switch parts[0] {
		case "inbound":
			if inbounds[parts[1]] == nil {
				inbounds[parts[1]] = &xray.Traffic{Tag: parts[1], IsInbound: true}
			}
			if parts[3] == "uplink" {
				inbounds[parts[1]].Up = stat.Value
			} else {
				inbounds[parts[1]].Down = stat.Value
			}
		case "user":
			if clients[parts[1]] == nil {
				clients[parts[1]] = &xray.ClientTraffic{Email: parts[1]}
			}
			if parts[3] == "uplink" {
				clients[parts[1]].Up = stat.Value
			} else {
				clients[parts[1]].Down = stat.Value
			}
		}
	}
	for _, value := range inbounds {
		inboundTraffic = append(inboundTraffic, value)
	}
	for _, value := range clients {
		clientTraffic = append(clientTraffic, value)
	}

	return
}
