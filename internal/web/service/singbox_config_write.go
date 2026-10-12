package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
)

// Check a candidate before replacing the file used by the running core. A
// failed check leaves both the existing config and the process untouched.
type nativeConfigCandidate struct {
	path, target     string
	managedOutbounds []map[string]any
}

func (c *nativeConfigCandidate) close() { _ = os.Remove(c.path) }

func (c *nativeConfigCandidate) publish(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(c.path, c.target); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(c.target))
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
	return candidate.publish(ctx)
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
	return &nativeConfigCandidate{path: name, target: path, managedOutbounds: cfg.ManagedOutbounds}, nil
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
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
