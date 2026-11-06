package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/mieru"
	"github.com/SawaMEN/3x-ui/v3/internal/sudoku"

	"github.com/SawaMEN/3x-ui/v3/internal/clientlimit"
	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/protocolserver"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Check a candidate before replacing the file used by the running core. A
// failed check leaves both the existing config and the process untouched.
type nativeConfigCandidate struct {
	speedLimitPlan   clientlimit.Plan
	path, target     string
	managedOutbounds []map[string]any
	protocolServers  []protocolserver.Plan
	nativeAWGTags    []string
	published        bool
}

func (c *nativeConfigCandidate) close() { _ = os.Remove(c.path) }

func (c *nativeConfigCandidate) publish(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(c.path, c.target); err != nil {
		return err
	}
	c.published = true
	return syncNativeConfigDir(c.target)
}

func syncNativeConfigDir(path string) error {
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func (s *SingBoxService) writeConfigCandidate(ctx context.Context) error {
	candidate, err := s.prepareConfigCandidate(ctx)
	if err != nil {
		return err
	}
	defer candidate.close()
	finish, err := candidate.stage(ctx, nil)
	if err != nil {
		return err
	}
	return finish(true)
}

// Capture disk state once. Applied bytes take precedence over an edited file
// when restoring a process that was already running.
type nativeConfigFileSnapshot struct {
	path   string
	data   []byte
	exists bool
}

func snapshotNativeConfig(path string, applied []byte) (*nativeConfigFileSnapshot, error) {
	snapshot := &nativeConfigFileSnapshot{path: path, data: applied, exists: applied != nil}
	if applied == nil {
		data, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		snapshot.data = data
		snapshot.exists = err == nil
	}
	return snapshot, nil
}
func (s *nativeConfigFileSnapshot) restore() error {
	if s.exists {
		return writeNativeConfigBytes(s.path, s.data)
	}
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return syncNativeConfigDir(s.path)
}

// Publication is provisional until the core and its sidecars have started.
func (c *nativeConfigCandidate) stage(ctx context.Context, applied []byte) (func(bool) error, error) {
	snapshot, err := snapshotNativeConfig(c.target, applied)
	if err != nil {
		return nil, err
	}
	finish := func(commit bool) error {
		if commit || !c.published {
			return nil
		}
		return snapshot.restore()
	}
	if err := c.publish(ctx); err != nil {
		return nil, errors.Join(err, finish(false))
	}
	return finish, nil
}

// Preparation is read/check only: do not publish or consume traffic until both
// the panel preflight and the actual binary have accepted the candidate.
func (s *SingBoxService) prepareConfigCandidate(ctx context.Context) (_ *nativeConfigCandidate, err error) {
	cfg, err := s.GetConfig()
	if err != nil {
		return nil, err
	}
	return s.prepareGeneratedConfigCandidate(ctx, cfg)
}

func (s *SingBoxService) prepareGeneratedConfigCandidate(ctx context.Context, cfg *singbox.Config) (_ *nativeConfigCandidate, err error) {
	binaries := map[string]bool{}
	for _, route := range cfg.SpeedLimitPlan.Routes {
		for _, alias := range route.Aliases {
			switch {
			case strings.HasPrefix(alias, "mieru:"):
				binaries[mieru.GetBinaryPath()] = true
			case strings.HasPrefix(alias, "sudoku:"):
				binaries[sudoku.GetBinaryPath(config.GetBinFolderPath())] = true
			case strings.HasPrefix(alias, "masque:"):
				binaries[singbox.GetBinaryPath()] = true
			}
		}
	}
	for binary := range binaries {
		if err := clientlimit.CheckBrokerBinary(ctx, binary); err != nil {
			return nil, err
		}
	}
	if err := protocolserver.Check(ctx, cfg.ProtocolServers); err != nil {
		return nil, err
	}
	data, err := cfg.Marshal()
	if err != nil {
		return nil, err
	}
	path := s.ProcessConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+s.coreType()+"-config-*.json")
	if err != nil {
		return nil, err
	}
	name := file.Name()
	defer file.Close()
	defer func() {
		if err != nil {
			_ = os.Remove(name)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return nil, err
	}
	if err := file.Sync(); err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, err
	}
	if err := s.candidate(name).Validate(ctx); err != nil {
		return nil, fmt.Errorf("validate candidate %s config: %w", s.coreType(), err)
	}
	var nativeAWGTags []string
	for _, endpoint := range cfg.Endpoints {
		if endpoint["type"] == "awg" {
			if tag, ok := endpoint["tag"].(string); ok {
				nativeAWGTags = append(nativeAWGTags, tag)
			}
		}
	}
	return &nativeConfigCandidate{speedLimitPlan: cfg.SpeedLimitPlan, path: name, target: path, managedOutbounds: cfg.ManagedOutbounds, protocolServers: cfg.ProtocolServers, nativeAWGTags: nativeAWGTags}, nil
}

// Restore a trusted applied snapshot with the same atomic/durable publication
// and permissions as a validated candidate. Callers must hold singBoxApplyMu.
func writeNativeConfigBytes(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".native-config-restore-*.json")
	if err != nil {
		return err
	}
	name := file.Name()
	defer os.Remove(name)
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return err
	}
	return syncNativeConfigDir(path)
}
