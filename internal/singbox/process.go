package singbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SawaMEN/3x-ui/v3/internal/clientlimit"

	gprocess "github.com/shirou/gopsutil/v4/process"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
)

const (
	defaultGracefulStopTimeout = 5 * time.Second
	defaultForceStopTimeout    = 2 * time.Second
	defaultCommandTimeout      = 10 * time.Second
)

func GetBinaryName() string { return filepath.Base(config.OwnerCoreBinaryPath()) }

func resolveManagedPath(path string) string {
	if path == "" || filepath.IsAbs(path) {
		return path
	}
	base := filepath.Dir(configExecutablePath())
	return filepath.Join(base, path)
}

func GetBinaryPath() string {
	return resolveManagedPath(filepath.Join(config.GetBinFolderPath(), GetBinaryName()))
}

func GetConfigPath() string {
	return resolveManagedPath(filepath.Join(config.GetBinFolderPath(), "hiddify-core.json"))
}

// resolvePath is kept as a small compatibility helper for callers that pass
// an already-managed relative path.
func resolvePath(path string) string {
	return resolveManagedPath(path)
}

func configExecutablePath() string {
	exe, err := os.Executable()
	if err != nil {
		return "."
	}
	return exe
}

type ProcessOptions struct {
	Xray             bool
	Name             string
	Binary           string
	RunCommand       string
	VersionPrefix    string
	RequiredVersion  string
	RequiredRevision string
	RequiredTags     []string
	RequiredAnyTags  []string
	NativeAPI        bool
	MASQUE           bool
}

func (p *Process) coreName() string {
	if p.profile != nil && p.profile.Name != "" {
		return p.profile.Name
	}
	return "sing-box"
}

type Process struct {
	profile       *ProcessOptions
	mu            sync.RWMutex
	lifecycle     sync.Mutex
	cmd           *exec.Cmd
	done          chan struct{}
	exitErr       error
	version       string
	startTime     time.Time
	config        string
	externalPID   int
	isolated      bool
	appliedConfig []byte
}

func NewProcessWithOptions(configPath string, options ProcessOptions, isolated bool) *Process {
	return &Process{config: resolvePath(configPath), version: "Unknown", profile: &options, isolated: isolated}
}

func (p *Process) binaryPath() string {
	if p.profile != nil {
		return resolvePath(p.profile.Binary)
	}
	return GetBinaryPath()
}

func (p *Process) runCommand() string {
	if p.profile != nil {
		return p.profile.RunCommand
	}
	return "srun"
}

func (p *Process) IsRunning() bool {
	p.mu.RLock()
	cmd, done, externalPID := p.cmd, p.done, p.externalPID
	p.mu.RUnlock()
	if cmd != nil && cmd.Process != nil {
		running := false
		if done != nil {
			select {
			case <-done:
			default:
				running = true
			}
		} else {
			running = true
		}
		if running {
			return true
		}
	}
	if p.isolated {
		return false
	}
	if externalPID > 0 {
		if processMatchesConfig(externalPID, p.binaryPath(), p.config, p.runCommand()) {
			return true
		}
		p.clearExternalPID(externalPID)
	}
	if pid := p.managedPID(); pid > 0 {
		p.mu.Lock()
		p.externalPID = pid
		p.mu.Unlock()
		p.rememberAppliedConfig()
		return true
	}
	return false
}

func (p *Process) clearExternalPID(pid int) {
	p.mu.Lock()
	if p.externalPID == pid {
		p.externalPID = 0
	}
	p.mu.Unlock()
}

func (p *Process) GetErr() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exitErr
}

func (p *Process) SetError(err error) {
	p.setErr(err)
}

func (p *Process) GetVersion() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.version
}

func (p *Process) SupportsNativeAPI() bool {
	return p.profile != nil && p.profile.NativeAPI
}

func (p *Process) SupportsMASQUE() bool {
	return p.profile != nil && p.profile.MASQUE
}

func (p *Process) GetStartTime() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.startTime
}

// RuntimeID remains stable when the panel adopts an existing process, and
// changes after a core restart (including PID reuse).
func (p *Process) RuntimeID() (string, error) {
	if !p.IsRunning() {
		return "", fmt.Errorf("%s is not running", p.coreName())
	}
	p.mu.RLock()
	pid := p.externalPID
	started, isolated := p.startTime, p.isolated
	owned := p.cmd != nil && p.cmd.Process != nil
	if owned && p.done != nil {
		select {
		case <-p.done:
			owned = false
		default:
		}
	}
	if owned {
		pid = p.cmd.Process.Pid
	} else {
		started = time.Time{}
	}
	p.mu.RUnlock()
	if pid <= 0 {
		return "", fmt.Errorf("%s process identity unavailable", p.coreName())
	}
	if isolated && !started.IsZero() {
		return fmt.Sprintf("isolated:%d:%d", pid, started.UnixNano()), nil
	}
	proc, err := gprocess.NewProcess(int32(pid))
	if err != nil {
		return "", err
	}
	created, err := proc.CreateTime()
	if err != nil {
		return "", err
	}
	if created <= 0 {
		return "", fmt.Errorf("invalid process creation time")
	}
	return fmt.Sprintf("%d:%d", pid, created), nil
}

// GetUptime returns the uptime of the sing-box process in seconds.
func (p *Process) GetUptime() uint64 {
	p.mu.RLock()
	startTime, externalPID, cmd, done := p.startTime, p.externalPID, p.cmd, p.done
	p.mu.RUnlock()

	if !startTime.IsZero() && cmd != nil && cmd.Process != nil {
		running := true
		if done != nil {
			select {
			case <-done:
				running = false
			default:
			}
		}
		if running {
			return uint64(time.Since(startTime).Seconds())
		}
	}
	if p.isolated {
		return 0
	}
	if externalPID > 0 && !processMatchesConfig(externalPID, p.binaryPath(), p.config, p.runCommand()) {
		p.clearExternalPID(externalPID)
		externalPID = 0
	}
	if externalPID <= 0 {
		externalPID = findRunningPID(p.binaryPath(), p.config, p.runCommand())
		if externalPID > 0 {
			p.mu.Lock()
			p.externalPID = externalPID
			p.mu.Unlock()
		}
	}
	if externalPID <= 0 {
		return 0
	}
	proc, err := gprocess.NewProcess(int32(externalPID))
	if err != nil {
		return 0
	}
	createdMs, err := proc.CreateTime()
	if err != nil || createdMs <= 0 {
		return 0
	}
	started := time.UnixMilli(createdMs)
	if started.After(time.Now()) {
		return 0
	}
	return uint64(time.Since(started).Seconds())
}

func (p *Process) ConfigPath() string { return p.config }

func (p *Process) Validate(ctx context.Context) error {
	binary := resolvePath(p.binaryPath())
	cfg := resolvePath(p.config)
	if _, err := os.Stat(binary); err != nil {
		err = fmt.Errorf("%s binary is not installed: %w", p.coreName(), err)
		p.setErr(err)
		return err
	}
	if _, err := os.Stat(cfg); err != nil {
		err = fmt.Errorf("%s config does not exist: %w", p.coreName(), err)
		p.setErr(err)
		return err
	}
	if data, err := os.ReadFile(cfg); err == nil && (configUsesProtocol(data, "masque-server") || configUsesProtocol(data, "masque-client")) {
		if _, err := p.Version(ctx); err != nil {
			return err
		}
		if !p.SupportsMASQUE() {
			return fmt.Errorf("MASQUE is unavailable in the configured Sing-box profile; installed: %s", p.GetVersion())
		}
	}
	if p.profile != nil {
		if _, err := p.Version(ctx); err != nil {
			return err
		}
	}
	cmdCtx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()
	args := []string{"check", "-c", cfg}
	if p.profile != nil && p.profile.Xray {
		args = []string{"run", "-test", "-c", cfg}
	}
	cmd := exec.CommandContext(cmdCtx, binary, args...)
	cmd.Dir = filepath.Dir(binary)
	cmd.Env = clientlimit.Default.Environment(os.Environ())
	output, err := cmd.CombinedOutput()
	if err != nil {
		err = fmt.Errorf("%s config check failed: %w: %s", p.coreName(), err, strings.TrimSpace(string(output)))
		p.setErr(err)
		return err
	}
	p.setErr(nil)
	return nil
}

func (p *Process) Version(ctx context.Context) (string, error) {
	binary := resolvePath(p.binaryPath())
	cmdCtx, cancel := context.WithTimeout(ctx, defaultCommandTimeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, binary, "version")
	cmd.Dir = filepath.Dir(binary)
	cmd.Env = clientlimit.Default.Environment(os.Environ())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s version failed: %w: %s", p.coreName(), err, strings.TrimSpace(string(output)))
	}
	version := strings.TrimSpace(string(output))
	if lines := strings.SplitN(version, "\n", 2); len(lines) > 0 {
		version = strings.TrimSpace(strings.TrimPrefix(lines[0], "sing-box version"))
	}
	if p.profile != nil {
		firstLine := strings.SplitN(string(output), "\n", 2)[0]
		if !strings.HasPrefix(firstLine, p.profile.VersionPrefix) {
			return "", fmt.Errorf("unexpected %s version output: %s", p.coreName(), strings.TrimSpace(firstLine))
		}
		fields := strings.Fields(strings.TrimPrefix(firstLine, p.profile.VersionPrefix))
		if len(fields) == 0 || (p.profile.RequiredVersion != "" && fields[0] != p.profile.RequiredVersion) {
			return "", fmt.Errorf("expected core %s, received %s", p.profile.RequiredVersion, strings.TrimSpace(string(output)))
		}
		version = fields[0]
		buildTags := map[string]bool{}
		var revision string
		for _, line := range strings.Split(string(output), "\n") {
			if tags, ok := strings.CutPrefix(line, "Tags: "); ok {
				for _, tag := range strings.Split(tags, ",") {
					buildTags[strings.TrimSpace(tag)] = true
				}
			}
			if value, ok := strings.CutPrefix(line, "Revision: "); ok {
				revision = strings.TrimSpace(value)
			}
		}
		if p.profile.RequiredRevision != "" && revision != p.profile.RequiredRevision {
			return "", fmt.Errorf("expected %s revision %s, received %q", p.coreName(), p.profile.RequiredRevision, revision)
		}
		for _, tag := range p.profile.RequiredTags {
			if !buildTags[tag] {
				return "", fmt.Errorf("core build lacks required tag %s", tag)
			}
		}
		if len(p.profile.RequiredAnyTags) > 0 {
			found := false
			for _, tag := range p.profile.RequiredAnyTags {
				found = found || buildTags[tag]
			}
			if !found {
				return "", fmt.Errorf("core build lacks one of required tags %v", p.profile.RequiredAnyTags)
			}
		}
	}
	p.mu.Lock()
	p.version = version
	p.mu.Unlock()
	return version, nil
}

func (p *Process) Start(ctx context.Context) error {
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	return p.startLocked(ctx)
}

func (p *Process) startLocked(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p.mu.RLock()
	running, done := p.cmd != nil && p.cmd.Process != nil, p.done
	p.mu.RUnlock()
	if running {
		select {
		case <-done:
		default:
			return nil
		}
	}
	if p.profile != nil {
		if _, err := p.Version(ctx); err != nil {
			p.setErr(err)
			return err
		}
	}
	if pid := p.managedPID(); pid > 0 {
		p.mu.Lock()
		p.externalPID = pid
		p.mu.Unlock()
		p.rememberAppliedConfig()
		return nil
	}
	if err := p.Validate(ctx); err != nil {
		return err
	}
	binary, cfg := resolvePath(p.binaryPath()), resolvePath(p.config)
	if err := ctx.Err(); err != nil {
		return err
	}
	// The core must outlive the HTTP request that started it.
	cmd := exec.CommandContext(context.Background(), binary, p.runCommand(), "-c", cfg)
	cmd.Dir = filepath.Dir(binary)
	cmd.Env = clientlimit.Default.Environment(os.Environ())
	output := &processOutput{}
	cmd.Stdout = output
	cmd.Stderr = output
	if err := cmd.Start(); err != nil {
		err = fmt.Errorf("start %s: %w", p.coreName(), err)
		p.setErr(err)
		return err
	}
	p.mu.Lock()
	p.cmd = cmd
	p.done = make(chan struct{})
	p.exitErr = nil
	p.externalPID = 0
	p.startTime = time.Now()
	done = p.done
	p.mu.Unlock()
	go func() {
		err := cmd.Wait()
		if err != nil {
			if details := output.String(); details != "" {
				err = fmt.Errorf("%w: %s", err, details)
			}
		}
		p.mu.Lock()
		p.exitErr = err
		close(done)
		p.mu.Unlock()
	}()
	// Startup errors (including bind collisions) happen after cmd.Start. Do not
	// report success while the child is still in that immediate failure window.
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-done:
		if err := p.GetErr(); err != nil {
			return fmt.Errorf("%s exited during startup: %w", p.coreName(), err)
		}
		return fmt.Errorf("%s exited during startup", p.coreName())
	case <-ctx.Done():
		_ = p.stopLocked()
		return ctx.Err()
	case <-timer.C:
	}
	if err := ctx.Err(); err != nil {
		_ = p.stopLocked()
		return err
	}
	p.rememberAppliedConfig()
	return nil
}

func (p *Process) setErr(err error) {
	p.mu.Lock()
	p.exitErr = err
	p.mu.Unlock()
}

func (p *Process) Stop() error {
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	return p.stopLocked()
}

func (p *Process) stopLocked() error {
	p.mu.RLock()
	cmd, done, externalPID := p.cmd, p.done, p.externalPID
	p.mu.RUnlock()
	if done != nil {
		select {
		case <-done:
			cmd = nil
		default:
		}
	}
	if cmd == nil || cmd.Process == nil {
		binary := p.binaryPath()
		if externalPID > 0 && !processMatchesConfig(externalPID, binary, p.config, p.runCommand()) {
			p.clearExternalPID(externalPID)
			externalPID = 0
		}
		if externalPID <= 0 && !p.isolated {
			externalPID = findRunningPID(binary, p.config, p.runCommand())
		}
		if externalPID <= 0 {
			return nil
		}
		if !processMatchesConfig(externalPID, binary, p.config, p.runCommand()) {
			p.clearExternalPID(externalPID)
			return nil
		}
		proc, err := os.FindProcess(externalPID)
		if err != nil {
			return err
		}
		if err := proc.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
			// Never escalate a cached PID without verifying it still points at
			// the configured sing-box binary. The PID may have been reused after
			// the identity check above.
			if !processMatchesConfig(externalPID, binary, p.config, p.runCommand()) {
				p.clearExternalPID(externalPID)
				return nil
			}
			if killErr := proc.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				return fmt.Errorf("force stop external %s process %d: %w", p.coreName(), externalPID, killErr)
			}
			if !p.waitForExternalExit(externalPID, defaultForceStopTimeout) {
				return fmt.Errorf("external %s process %d did not exit after force stop", p.coreName(), externalPID)
			}
			p.setErr(nil)
			return nil
		}
		if p.waitForExternalExit(externalPID, defaultGracefulStopTimeout) {
			p.setErr(nil)
			return nil
		}
		// Graceful shutdown timed out. Revalidate immediately before SIGKILL so
		// a PID recycled during the wait cannot cause an unrelated process to be
		// terminated.
		if !processMatchesConfig(externalPID, binary, p.config, p.runCommand()) {
			p.clearExternalPID(externalPID)
			p.setErr(nil)
			return nil
		}
		proc, err = os.FindProcess(externalPID)
		if err != nil {
			return err
		}
		if err := proc.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("force stop external %s process %d: %w", p.coreName(), externalPID, err)
		}
		if !p.waitForExternalExit(externalPID, defaultForceStopTimeout) {
			return fmt.Errorf("external %s process %d did not exit after force stop", p.coreName(), externalPID)
		}
		p.setErr(nil)
		return nil
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("force stop %s: %w", p.coreName(), err)
		}
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(defaultGracefulStopTimeout):
			if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
				return fmt.Errorf("force stop %s: %w", p.coreName(), err)
			}
			select {
			case <-done:
			case <-time.After(defaultForceStopTimeout):
				return fmt.Errorf("%s did not exit after force stop", p.coreName())
			}
		}
	}
	p.setErr(nil)
	return nil
}

func (p *Process) Restart(ctx context.Context) error {
	p.lifecycle.Lock()
	defer p.lifecycle.Unlock()
	if err := p.Validate(ctx); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.stopLocked(); err != nil {
		return err
	}
	return p.startLocked(ctx)
}

func (p *Process) waitForExternalExit(pid int, timeout time.Duration) bool {
	binary := p.binaryPath()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !processMatchesConfig(pid, binary, p.config, p.runCommand()) {
			p.clearExternalPID(pid)
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !processMatchesConfig(pid, binary, p.config, p.runCommand()) {
		p.clearExternalPID(pid)
		return true
	}
	return false
}

func processMatchesBinary(pid int, binary string) bool {
	if pid <= 0 || runtime.GOOS != "linux" {
		return false
	}

	binary = filepath.Clean(resolvePath(binary))
	if resolved, err := filepath.EvalSymlinks(binary); err == nil {
		binary = filepath.Clean(resolved)
	}

	procDir := filepath.Join("/proc", strconv.Itoa(pid))
	if exe, err := os.Readlink(filepath.Join(procDir, "exe")); err == nil {
		exe = strings.TrimSuffix(exe, " (deleted)")
		return filepath.Clean(exe) == binary
	}

	// /proc/<pid>/exe can be unavailable under restrictive procfs settings.
	// Fall back to argv[0], but only when the stronger executable check could
	// not be read at all.
	cmdline, err := os.ReadFile(filepath.Join(procDir, "cmdline"))
	if err != nil {
		return false
	}
	args := strings.Split(string(cmdline), "\x00")
	if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
		return false
	}
	arg0 := filepath.Clean(resolvePath(args[0]))
	if resolved, err := filepath.EvalSymlinks(arg0); err == nil {
		arg0 = filepath.Clean(resolved)
	}
	return arg0 == binary
}

func findRunningPID(binary string, configs ...string) int {
	if runtime.GOOS != "linux" {
		return 0
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		if processMatchesBinary(pid, binary) {
			if len(configs) > 0 && !processMatchesConfig(pid, binary, configs[0], configs[1:]...) {
				continue
			}
			return pid
		}
	}
	return 0
}

const processOutputLimit = 64 * 1024

type processOutput struct {
	mu  sync.Mutex
	buf []byte
}

func (w *processOutput) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(b) >= processOutputLimit {
		w.buf = append(w.buf[:0], b[len(b)-processOutputLimit:]...)
		return len(b), nil
	}
	w.buf = append(w.buf, b...)
	if len(w.buf) > processOutputLimit {
		w.buf = append([]byte(nil), w.buf[len(w.buf)-processOutputLimit:]...)
	}
	return len(b), nil
}

func (w *processOutput) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return strings.TrimSpace(string(w.buf))
}

func (p *Process) managedPID() int {
	if p.isolated {
		return 0
	}
	return findRunningPID(p.binaryPath(), p.config, p.runCommand())
}

func configUsesProtocol(data []byte, protocol string) bool {
	var cfg struct {
		Endpoints []struct {
			Type string `json:"type"`
		} `json:"endpoints"`
		Inbounds []struct {
			Type string `json:"type"`
		} `json:"inbounds"`
		Outbounds []struct {
			Type string `json:"type"`
		} `json:"outbounds"`
	}
	if json.Unmarshal(data, &cfg) != nil {
		return false
	}
	for _, endpoint := range cfg.Endpoints {
		if endpoint.Type == protocol {
			return true
		}
	}
	for _, inbound := range cfg.Inbounds {
		if inbound.Type == protocol {
			return true
		}
	}
	for _, outbound := range cfg.Outbounds {
		if outbound.Type == protocol {
			return true
		}
	}
	return false
}
