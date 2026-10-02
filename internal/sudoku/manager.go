package sudoku

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

type Instance struct {
	ID     int
	Tag    string
	Config Config
}

type managed struct {
	proc              *Process
	fingerprint       string
	binary            string
	binaryFingerprint string
}

type Manager struct {
	mu      sync.Mutex
	procs   map[int]*managed
	lastErr map[int]string
}

var (
	managerOnce      sync.Once
	managerSingleton *Manager
)

func GetManager() *Manager {
	managerOnce.Do(func() {
		managerSingleton = &Manager{
			procs:   make(map[int]*managed),
			lastErr: make(map[int]string),
		}
	})
	return managerSingleton
}

func (m *Manager) Reconcile(ctx context.Context, desired []Instance) {
	m.mu.Lock()
	defer m.mu.Unlock()

	binDir := config.GetBinFolderPath()
	if !IsInstalled(binDir) {
		for id, cur := range m.procs {
			if !m.stopTracked(id, cur, "binary unavailable") {
				continue
			}
			delete(m.procs, id)
			delete(m.lastErr, id)
			_ = os.Remove(configPath(binDir, id))
		}
		return
	}
	binary := GetBinaryPath(binDir)
	binaryFP := currentBinaryFingerprint(binary)

	want := make(map[int]Instance, len(desired))
	for _, inst := range desired {
		want[inst.ID] = inst
	}

	for id, cur := range m.procs {
		if _, ok := want[id]; ok {
			continue
		}
		if !m.stopTracked(id, cur, "inbound removed or disabled") {
			continue
		}
		delete(m.procs, id)
		delete(m.lastErr, id)
		_ = os.Remove(configPath(binDir, id))
	}

	for _, inst := range desired {
		fp := fingerprint(inst.Config)
		cur := m.procs[inst.ID]
		if cur != nil && cur.proc != nil && cur.proc.IsRunning() && cur.fingerprint == fp && cur.binaryFingerprint == binaryFP {
			continue
		}

		if cur != nil {
			if !m.stopTracked(inst.ID, cur, "restart") {
				continue
			}
			delete(m.procs, inst.ID)
		}

		if err := writeConfig(binDir, inst.ID, inst.Config); err != nil {
			m.recordError(inst.ID, fmt.Errorf("write config: %w", err), "sudoku: failed to write config for inbound %d (%s): %v", inst.ID, inst.Tag, err)
			continue
		}
		proc := NewProcess(configPath(binDir, inst.ID), inst.Tag)
		if err := proc.Start(binary); err != nil {
			m.recordError(inst.ID, err, "sudoku: failed to start inbound %d (%s): %v", inst.ID, inst.Tag, err)
			continue
		}
		m.procs[inst.ID] = &managed{
			proc:              proc,
			fingerprint:       fp,
			binary:            binary,
			binaryFingerprint: binaryFP,
		}
		delete(m.lastErr, inst.ID)
		logger.Infof("sudoku: started inbound %d (%s)", inst.ID, inst.Tag)
	}
}

func (m *Manager) stopTracked(id int, cur *managed, reason string) bool {
	if cur == nil || cur.proc == nil {
		return true
	}
	if err := cur.proc.Stop(); err != nil {
		wrapped := fmt.Errorf("stop Sudoku process (%s): %w", reason, err)
		m.recordError(id, wrapped, "sudoku: failed to stop inbound %d during %s: %v", id, reason, err)
		return false
	}
	return true
}

func (m *Manager) recordError(id int, err error, format string, args ...any) {
	message := err.Error()
	if m.lastErr[id] == message {
		return
	}
	m.lastErr[id] = message
	logger.Warningf(format, args...)
}

func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()
	binDir := config.GetBinFolderPath()
	for id, cur := range m.procs {
		if !m.stopTracked(id, cur, "manager shutdown") {
			continue
		}
		delete(m.procs, id)
		delete(m.lastErr, id)
		_ = os.Remove(configPath(binDir, id))
	}
}

func (m *Manager) Remove(id int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	binDir := config.GetBinFolderPath()
	if cur := m.procs[id]; cur != nil {
		if !m.stopTracked(id, cur, "inbound removal") {
			return
		}
		delete(m.procs, id)
	}
	delete(m.lastErr, id)
	_ = os.Remove(configPath(binDir, id))
	_ = os.Remove(keyPath(binDir, id))
}

func currentBinaryFingerprint(path string) string {
	stat, err := os.Stat(path)
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%d:%d", stat.Size(), stat.ModTime().UnixNano())
}

func fingerprint(cfg Config) string {
	b, _ := json.Marshal(cfg)
	return string(b)
}

func writeConfig(binDir string, id int, cfg Config) error {
	dir := configDir(binDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	path := configPath(binDir, id)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o640)
}

func WriteMasterKey(binDir string, id int, key string) error {
	dir := configDir(binDir)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	return os.WriteFile(keyPath(binDir, id), []byte(key+"\n"), 0o600)
}

func ReadMasterKey(binDir string, id int) (string, error) {
	b, err := os.ReadFile(keyPath(binDir, id))
	if err != nil {
		return "", err
	}
	return stringTrim(b), nil
}

func stringTrim(b []byte) string {
	end := len(b)
	for end > 0 && (b[end-1] == '\n' || b[end-1] == '\r' || b[end-1] == ' ' || b[end-1] == '\t') {
		end--
	}
	start := 0
	for start < end && (b[start] == ' ' || b[start] == '\t') {
		start++
	}
	return string(b[start:end])
}
