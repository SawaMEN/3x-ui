package service

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

var hiddifyRuntime = hiddify.NewRuntime(hiddify.GetConfigPath(), false)
var hiddifyProcess = hiddifyRuntime.Process
var hiddifyNeedRestart atomic.Bool
var hiddifyManuallyStopped atomic.Bool
var hiddifyStats = hiddify.NewStatsClient()

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

// Non-destructive snapshots can be retried after an RPC or database failure.
func (s *SingBoxService) pollHiddifyTraffic(ctx context.Context) error {
	singBoxTrafficMu.Lock()
	defer singBoxTrafficMu.Unlock()
	epoch, err := s.process().RuntimeID()
	if err != nil {
		return err
	}
	stats, err := hiddifyStats.Query(ctx, nil, false)
	if err != nil {
		return fmt.Errorf("hiddify user statistics: %w", err)
	}
	after, err := s.process().RuntimeID()
	if err != nil {
		return err
	}
	if epoch != after {
		return fmt.Errorf("hiddify core restarted during statistics query")
	}
	if err := s.commitHiddifySnapshot(epoch, stats); err != nil {
		return err
	}
	if err := s.enforceHiddifySessions(ctx); err != nil {
		logger.Warning("hiddify session revocation failed; retry after config apply:", err)
	}
	return nil
}

func (s *SingBoxService) hiddifySessions(ctx context.Context) ([]singbox.ActiveSession, error) {
	return hiddify.ActiveSessions(ctx)
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
	return hiddify.DisconnectSessions(ctx, match)
}

func splitHiddifyStats(stats []singbox.V2RayStat) (inboundTraffic []*coremodel.Traffic, clientTraffic []*coremodel.ClientTraffic) {
	inbounds := map[string]*coremodel.Traffic{}
	clients := map[string]*coremodel.ClientTraffic{}
	for _, stat := range stats {
		kind, rest, _ := strings.Cut(stat.Name, ">>>")
		i := strings.LastIndex(rest, ">>>traffic>>>")
		identity, direction, found := "", "", i > 0
		if found {
			identity, direction = rest[:i], rest[i+len(">>>traffic>>>"):]
		}
		if !found || identity == "" || (direction != "uplink" && direction != "downlink") || stat.Value <= 0 {
			continue
		}
		switch kind {
		case "inbound":
			if inbounds[identity] == nil {
				inbounds[identity] = &coremodel.Traffic{Tag: identity, IsInbound: true}
			}
			if direction == "uplink" {
				inbounds[identity].Up = stat.Value
			} else {
				inbounds[identity].Down = stat.Value
			}
		case "user":
			if clients[identity] == nil {
				clients[identity] = &coremodel.ClientTraffic{Email: identity}
			}
			if direction == "uplink" {
				clients[identity].Up = sumTrafficDelta(clients[identity].Up, stat.Value)
			} else {
				clients[identity].Down = sumTrafficDelta(clients[identity].Down, stat.Value)
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
