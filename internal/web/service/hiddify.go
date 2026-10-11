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

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

var hiddifyRuntime = hiddify.NewRuntime(hiddify.GetConfigPath(), false)
var hiddifyProcess = hiddifyRuntime.Process
var hiddifyNeedRestart atomic.Bool
var hiddifyManuallyStopped atomic.Bool
var hiddifyStats = hiddify.NewStatsClient()
var hiddifyPendingInbounds []*coremodel.Traffic
var hiddifyPendingClients []*coremodel.ClientTraffic

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

func splitHiddifyStats(stats []singbox.V2RayStat) (inboundTraffic []*coremodel.Traffic, clientTraffic []*coremodel.ClientTraffic) {
	inbounds := map[string]*coremodel.Traffic{}
	clients := map[string]*coremodel.ClientTraffic{}
	for _, stat := range stats {
		parts := strings.Split(stat.Name, ">>>")
		if len(parts) != 4 || parts[2] != "traffic" || (parts[3] != "uplink" && parts[3] != "downlink") || stat.Value <= 0 {
			continue
		}
		switch parts[0] {
		case "inbound":
			if inbounds[parts[1]] == nil {
				inbounds[parts[1]] = &coremodel.Traffic{Tag: parts[1], IsInbound: true}
			}
			if parts[3] == "uplink" {
				inbounds[parts[1]].Up = stat.Value
			} else {
				inbounds[parts[1]].Down = stat.Value
			}
		case "user":
			if clients[parts[1]] == nil {
				clients[parts[1]] = &coremodel.ClientTraffic{Email: parts[1]}
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

func (s *SingBoxService) startProcess(ctx context.Context) error {
	if s.Hiddify {
		return hiddifyRuntime.Start(ctx)
	}
	return s.process().Start(ctx)
}
func (s *SingBoxService) restartProcess(ctx context.Context) error {
	if s.Hiddify {
		return hiddifyRuntime.Restart(ctx)
	}
	return s.process().Restart(ctx)
}
func (s *SingBoxService) stopProcess(ctx context.Context) error {
	if s.Hiddify {
		return hiddifyRuntime.Stop(ctx)
	}
	return s.process().Stop()
}
