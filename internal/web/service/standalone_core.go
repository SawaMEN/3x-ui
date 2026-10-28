package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/coreinstall"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

var (
	upstreamProcess                                = standaloneProcess(CoreTypeSingBoxUpstream, standaloneConfigPath(CoreTypeSingBoxUpstream), false)
	standaloneXrayProcess                          = standaloneProcess(CoreTypeXray, standaloneConfigPath(CoreTypeXray), false)
	upstreamNeedRestart, standaloneXrayNeedRestart atomic.Bool
	upstreamStopped, standaloneXrayStopped         atomic.Bool
)

func standaloneBinaryPath(engine string) string {
	return config.StandaloneCoreBinaryPath(engine)
}
func standaloneConfigPath(engine string) string {
	return filepath.Join(filepath.Dir(config.OwnerCoreBinaryPath()), engine+".json")
}
func standaloneProcess(engine, path string, isolated bool) *singbox.Process {
	profile := singbox.ProcessOptions{Name: engine, Binary: standaloneBinaryPath(engine), RunCommand: "run", VersionPrefix: "sing-box version "}
	if engine == CoreTypeXray {
		profile.VersionPrefix = "Xray "
		profile.Xray = true
	}
	return singbox.NewProcessWithOptions(path, profile, isolated)
}

func InstallStandaloneCore(ctx context.Context, engine, version string) (string, error) {
	if engine != CoreTypeXray && engine != CoreTypeSingBoxUpstream {
		return "", fmt.Errorf("unsupported standalone core %q", engine)
	}
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	svc := NativeCore(engine)
	if svc.IsRunning() {
		return "", fmt.Errorf("switch to another core before updating %s", engine)
	}
	arm := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "GOARM" {
				arm = strings.Split(setting.Value, ",")[0]
			}
		}
	}
	return (coreinstall.Installer{}).Install(ctx, engine, version, runtime.GOOS, runtime.GOARCH, arm, svc.BinaryPath(), func(path string) error {
		profile := singbox.ProcessOptions{Name: engine, Binary: path, RunCommand: "run", VersionPrefix: "sing-box version "}
		if engine == CoreTypeXray {
			profile.VersionPrefix = "Xray "
			profile.Xray = true
		}
		p := singbox.NewProcessWithOptions("", profile, true)
		_, err := p.Version(ctx)
		return err
	})
}

func (s *SingBoxService) writeStandaloneXrayConfig(ctx context.Context) error {
	cfg, err := (&CoreService{}).GetXrayConfig()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	path := s.ProcessConfigPath()
	if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".xray-candidate-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = s.candidate(f.Name()).Validate(ctx); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func (s *SingBoxService) restartStandaloneXray(ctx context.Context) error {
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	return s.restartStandaloneXrayLocked(ctx)
}
func (s *SingBoxService) restartStandaloneXrayLocked(ctx context.Context) error {
	old := s.process().AppliedConfig()
	if err := s.writeStandaloneXrayConfig(ctx); err != nil {
		s.process().SetError(err)
		return err
	}
	if s.IsRunning() {
		_ = s.pollStandaloneTraffic(ctx)
	}
	if err := s.process().Restart(ctx); err != nil {
		if len(old) > 0 {
			restoreErr := writeNativeConfigBytes(s.ProcessConfigPath(), old)
			if restoreErr == nil {
				restoreErr = s.process().Start(context.Background())
			}
			return errors.Join(err, restoreErr)
		}
		return err
	}
	s.stoppedFlag().Store(false)
	return nil
}
func (s *SingBoxService) pollStandaloneTraffic(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	ownerTrafficMu.Lock()
	defer ownerTrafficMu.Unlock()
	epoch, err := s.process().RuntimeID()
	if err != nil {
		return err
	}
	if s.coreType() == CoreTypeSingBoxUpstream {
		// Official binaries have no persistent user-counter API. Never infer
		// quota counters from live Clash connections: closed flows are absent.
		return s.pollProtocolServerTraffic(ctx)
	}
	var client *singbox.V2RayStatsClient
	if s.coreType() == CoreTypeXray {
		port := reservedAPIPort()
		client = singbox.NewV2RayStatsClient(fmt.Sprintf("127.0.0.1:%d", port), "/xray.app.stats.command.StatsService/QueryStats")
		defer client.Close()
	}
	stats, err := client.Query(ctx, nil, false)
	if err != nil {
		return err
	}
	after, err := s.process().RuntimeID()
	if err != nil {
		return err
	}
	if after != epoch {
		return fmt.Errorf("core restarted during statistics query")
	}
	return s.commitHiddifySnapshot(epoch, stats, s.coreType()+":")
}

func (s *SingBoxService) validateStandaloneXrayBytes(ctx context.Context, data []byte) error {
	dir := filepath.Dir(s.ProcessConfigPath())
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".xray-check-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(data); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return s.candidate(f.Name()).Validate(ctx)
}
