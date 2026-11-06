package service

import (
	"context"
	"fmt"
	"net"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/SawaMEN/3x-ui/v3/internal/coremodel"
	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
	"github.com/SawaMEN/3x-ui/v3/internal/protocolserver"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

var (
	hiddifyRuntime         = hiddify.NewRuntime(hiddify.GetConfigPath(), false)
	hiddifyProcess         = hiddifyRuntime.Process
	ownerTrafficMu         sync.Mutex
	hiddifyNeedRestart     atomic.Bool
	hiddifyManuallyStopped atomic.Bool
	hiddifyStats           = hiddify.NewStatsClient()
)

// InitializeNativeCoreRuntime resolves paths after the service environment has
// been loaded. Call once at process startup, before any service goroutines.
func InitializeNativeCoreRuntime() {
	hiddifyRuntime = hiddify.NewRuntime(hiddify.GetConfigPath(), false)
	hiddifyProcess = hiddifyRuntime.Process
	upstreamProcess = standaloneProcess(CoreTypeSingBoxUpstream, standaloneConfigPath(CoreTypeSingBoxUpstream), false)
	standaloneXrayProcess = standaloneProcess(CoreTypeXray, standaloneConfigPath(CoreTypeXray), false)
}

func IsNativeCore(core string) bool {
	return core == CoreTypeSingBox || core == legacyCoreTypeHiddify || core == CoreTypeSingBoxUpstream
}

func NativeCore(core string) *SingBoxService {
	// Both names resolve to the owner’s modified sing-box. The old name is
	// accepted only for compatibility with persisted settings and older nodes.
	return &SingBoxService{engine: canonicalCoreType(core)}
}

func SelectedNativeCore() *SingBoxService {
	core, _ := (&SettingService{}).GetCoreType()
	return NativeCore(core)
}
func (s *SingBoxService) CoreType() string { return s.coreType() }
func (s *SingBoxService) coreType() string {
	if s.engine != "" {
		return s.engine
	}
	return CoreTypeSingBox
}

func (s *SingBoxService) process() *singbox.Process {
	if s.coreType() == CoreTypeSingBoxUpstream {
		return upstreamProcess
	}
	if s.coreType() == CoreTypeXray {
		return standaloneXrayProcess
	}
	return hiddifyProcess
}

func (s *SingBoxService) candidate(path string) *singbox.Process {
	if s.coreType() != CoreTypeSingBox {
		return standaloneProcess(s.coreType(), path, true)
	}
	return hiddify.NewProcess(path, true)
}

func (s *SingBoxService) translateInbound(raw map[string]any) (map[string]any, error) {
	{
		if s.coreType() == CoreTypeSingBoxUpstream {
			return singbox.TranslateXrayInbound(raw)
		}
		return hiddify.TranslateInbound(raw)
	}
}

func (s *SingBoxService) translateOutbound(raw map[string]any) (map[string]any, error) {
	{
		if s.coreType() == CoreTypeSingBoxUpstream {
			return singbox.TranslateXrayOutbound(raw)
		}
		return hiddify.TranslateOutbound(raw)
	}
}

func (s *SingBoxService) restartFlag() *atomic.Bool {
	{
		if s.coreType() == CoreTypeSingBoxUpstream {
			return &upstreamNeedRestart
		}
		if s.coreType() == CoreTypeXray {
			return &standaloneXrayNeedRestart
		}
		return &hiddifyNeedRestart
	}
}

func (s *SingBoxService) stoppedFlag() *atomic.Bool {
	{
		if s.coreType() == CoreTypeSingBoxUpstream {
			return &upstreamStopped
		}
		if s.coreType() == CoreTypeXray {
			return &standaloneXrayStopped
		}
		return &hiddifyManuallyStopped
	}
}

func (s *SingBoxService) logPath() string {
	return filepath.Join(filepath.Dir(s.ProcessConfigPath()), s.coreType()+".log")
}

// Non-destructive snapshots can be retried after an RPC or database failure.
func (s *SingBoxService) pollHiddifyTraffic(ctx context.Context) error {
	ownerTrafficMu.Lock()
	defer ownerTrafficMu.Unlock()
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
	if err := s.pollProtocolServerTraffic(ctx); err != nil {
		return err
	}
	if err := s.enforceHiddifySessions(ctx); err != nil {
		logger.Warning("hiddify session revocation failed; retry after config apply:", err)
	}
	return nil
}

func (s *SingBoxService) hiddifySessions(ctx context.Context) ([]singbox.ActiveSession, error) {
	if s.coreType() == CoreTypeSingBoxUpstream {
		connections, err := singbox.NewClashStatsClient().Connections(ctx)
		if err != nil {
			return nil, err
		}
		sessions := make([]singbox.ActiveSession, 0, len(connections))
		for _, c := range connections {
			sessions = append(sessions, singbox.ActiveSession{
				ID:          c.ID,
				Core:        s.coreType(),
				Inbound:     c.Metadata.InboundName,
				User:        c.Metadata.User,
				Network:     c.Metadata.Network,
				Source:      net.JoinHostPort(c.Metadata.SourceIP, c.Metadata.SourcePort),
				Destination: net.JoinHostPort(c.Metadata.DestinationIP, c.Metadata.DestinationPort),
				Domain:      c.Metadata.Host,
				CreatedAt:   c.Start.Unix(),
				Upload:      c.Upload,
				Download:    c.Download,
				Chain:       c.Chains,
			})
		}
		return sessions, nil
	}
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
	extra, extraTags, err := protocolserver.Presence(ctx)
	if err != nil {
		return nil, nil, err
	}
	for email, sources := range extra {
		if online[email] == nil {
			online[email] = map[string]struct{}{}
		}
		for source := range sources {
			online[email][source] = struct{}{}
		}
	}
	for _, tag := range extraTags {
		tags[tag] = true
	}
	active := []string{}
	for tag := range tags {
		active = append(active, tag)
	}
	sort.Strings(active)
	return online, active, nil
}

func (s *SingBoxService) disconnectHiddify(ctx context.Context, match func(singbox.ActiveSession) bool) (int, error) {
	if s.coreType() == CoreTypeSingBoxUpstream {
		sessions, err := s.hiddifySessions(ctx)
		if err != nil {
			return 0, err
		}
		client := singbox.NewClashStatsClient()
		count := 0
		for _, session := range sessions {
			if match(session) {
				if err := client.CloseConnection(ctx, session.ID); err != nil {
					return count, err
				}
				count++
			}
		}
		return count, nil
	}
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
	if s.coreType() != CoreTypeSingBox {
		return s.process().Start(ctx)
	}
	{
		return hiddifyRuntime.Start(ctx)
	}
}

func (s *SingBoxService) restartProcess(ctx context.Context) error {
	if s.coreType() != CoreTypeSingBox {
		return s.process().Restart(ctx)
	}
	{
		return hiddifyRuntime.Restart(ctx)
	}
}

func (s *SingBoxService) stopProcess(ctx context.Context) error {
	if s.coreType() != CoreTypeSingBox {
		return s.process().Stop()
	}
	{
		return hiddifyRuntime.Stop(ctx)
	}
}

func (s *SingBoxService) templateKey() string {
	if s.coreType() == CoreTypeSingBoxUpstream {
		return "singBoxUpstreamConfigTemplate"
	}
	return "singBoxConfigTemplate"
}
