package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// Check a candidate before replacing the file used by the running core. A
// failed check leaves both the existing config and the process untouched.
func (s *SingBoxService) writeConfigCandidate(ctx context.Context) error {
	cfg, err := s.GetConfig()
	if err != nil {
		return err
	}
	data, err := cfg.Marshal()
	if err != nil {
		return err
	}
	path := s.ProcessConfigPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+s.coreType()+"-config-*.json")
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
	if err := s.candidate(name).Validate(ctx); err != nil {
		return fmt.Errorf("validate candidate %s config: %w", s.coreType(), err)
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
