package service

import (
	"context"
	"debug/buildinfo"
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
	switch engine {
	case CoreTypeXray:
		return filepath.Join(filepath.Dir(config.OwnerCoreBinaryPath()), "xray.json")
	case CoreTypeSingBoxUpstream:
		return filepath.Join(filepath.Dir(config.OwnerCoreBinaryPath()), "sing-box-upstream.json")
	default:
		return ""
	}
}

func standaloneProcess(engine, path string, isolated bool) *singbox.Process {
	return singbox.NewProcessWithOptions(path, standaloneProfile(engine, standaloneBinaryPath(engine)), isolated)
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
	if err := cleanupStandaloneCoreArtifacts(engine, svc.BinaryPath(), false); err != nil {
		return "", err
	}
	return installStandaloneAt(ctx, engine, version, svc.BinaryPath())
}

func standaloneProfile(engine, binary string) singbox.ProcessOptions {
	profile := singbox.ProcessOptions{Name: engine, Binary: binary, RunCommand: "run", VersionPrefix: "sing-box version "}
	if engine == CoreTypeXray {
		profile.VersionPrefix = "Xray "
		profile.Xray = true
	}
	return profile
}

func standaloneARM() string {
	arm := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "GOARM" {
				arm = strings.Split(setting.Value, ",")[0]
			}
		}
	}
	return arm
}

func StandaloneCoreVersions(ctx context.Context, engine string) ([]coreinstall.Version, error) {
	return (coreinstall.Installer{}).Versions(ctx, engine, runtime.GOOS, runtime.GOARCH, standaloneARM())
}

func installStandaloneAt(ctx context.Context, engine, version, target string) (string, error) {
	return (coreinstall.Installer{}).Install(ctx, engine, version, runtime.GOOS, runtime.GOARCH, standaloneARM(), target, func(path string) error {
		profile := standaloneProfile(engine, path)
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
	if err = os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
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

// UninstallStandaloneCore preserves configuration and shared geodata for reinstall.
func UninstallStandaloneCore(ctx context.Context, engine string) error {
	if engine != CoreTypeXray && engine != CoreTypeSingBoxUpstream {
		return fmt.Errorf("the bundled core cannot be removed")
	}
	singBoxApplyMu.Lock()
	defer singBoxApplyMu.Unlock()
	selected, err := (&SettingService{}).GetCoreType()
	if err != nil {
		return err
	}
	svc := NativeCore(engine)
	if selected == engine || svc.IsRunning() {
		return fmt.Errorf("switch to another core before removing %s", engine)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(svc.BinaryPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := cleanupStandaloneCoreArtifacts(engine, svc.BinaryPath(), true); err != nil {
		return err
	}
	svc.process().SetError(nil)
	return nil
}

func cleanupStandaloneCoreArtifacts(engine, target string, removeBackups bool) error {
	dir := filepath.Dir(target)
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	prefixes := []string{".core-" + engine + "-install-", ".core-" + engine + "-update-", ".core-install-", ".core-update-", ".core-geodata-"}
	if removeBackups {
		prefixes = append(prefixes, ".core-"+engine+"-backup-")
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// Older backups have no engine in their name; inspect metadata without
		// executing them, and retain unrecognized recovery copies.
		if removeBackups && strings.HasPrefix(entry.Name(), ".core-backup-") {
			file := filepath.Join(dir, entry.Name())
			info, err := buildinfo.ReadFile(file)
			if err == nil && ((engine == CoreTypeXray && info.Main.Path == "github.com/xtls/xray-core") || (engine == CoreTypeSingBoxUpstream && info.Main.Path == "github.com/sagernet/sing-box")) {
				if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("remove old %s backup: %w", engine, err)
				}
			}
			continue
		}
		for _, prefix := range prefixes {
			if !strings.HasPrefix(entry.Name(), prefix) {
				continue
			}
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("remove stale %s payload: %w", engine, err)
			}
			break
		}
	}
	return nil
}

var coreSwitchInProgress atomic.Int32

func BeginCoreSwitch() func() {
	coreSwitchInProgress.Add(1)
	return func() { coreSwitchInProgress.Add(-1) }
}
func CoreSwitchInProgress() bool { return coreSwitchInProgress.Load() > 0 }
