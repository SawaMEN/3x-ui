package sudoku

import (
    "context"
    "errors"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "sync"
    "sync/atomic"
    "syscall"
    "time"

    "github.com/SawaMEN/3x-ui/v3/internal/logger"
)

type Process struct {
    mu sync.RWMutex
    cmd *exec.Cmd
    done chan struct{}
    configPath string
    label string
    exitErr error
    intentionalStop atomic.Bool
}

func NewProcess(configPath, label string) *Process {
    return &Process{configPath: configPath, label: label}
}

func (p *Process) IsRunning() bool {
    p.mu.RLock()
    cmd, done := p.cmd, p.done
    p.mu.RUnlock()
    if cmd == nil || cmd.Process == nil {
        return false
    }
    if done != nil {
        select {
        case <-done:
            return false
        default:
        }
    }
    return true
}

func (p *Process) Start(binary string) error {
    if p.IsRunning() {
        return errors.New("Sudoku is already running")
    }
    cmd := exec.CommandContext(context.Background(), binary, "-c", p.configPath)
    cmd.Env = append(os.Environ(), "SUDOKU_LOG_NO_TIMESTAMP=true")
    lw := &logWriter{label: p.label}
    cmd.Stdout = lw
    cmd.Stderr = lw
    done := make(chan struct{})
    p.mu.Lock()
    p.cmd, p.done, p.exitErr = cmd, done, nil
    p.mu.Unlock()
    p.intentionalStop.Store(false)

    if err := cmd.Start(); err != nil {
        close(done)
        p.mu.Lock()
        p.cmd = nil
        p.mu.Unlock()
        return err
    }
    go p.wait(cmd, done)
    return nil
}

func (p *Process) wait(cmd *exec.Cmd, done chan struct{}) {
    defer close(done)
    err := cmd.Wait()
    if err == nil || p.intentionalStop.Load() {
        return
    }
    logger.Errorf("sudoku: %s exited: %v", p.label, err)
    p.mu.Lock()
    p.exitErr = err
    p.mu.Unlock()
}

func (p *Process) Stop() error {
    if !p.IsRunning() {
        return nil
    }
    p.intentionalStop.Store(true)
    p.mu.RLock()
    cmd, done := p.cmd, p.done
    p.mu.RUnlock()
    if cmd == nil || cmd.Process == nil {
        return nil
    }
    if runtime.GOOS == "windows" {
        _ = cmd.Process.Kill()
        return waitForExit(done, 2*time.Second)
    }
    if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
        return err
    }
    if err := waitForExit(done, 5*time.Second); err == nil {
        return nil
    }
    _ = cmd.Process.Kill()
    return waitForExit(done, 2*time.Second)
}

func waitForExit(done <-chan struct{}, timeout time.Duration) error {
    timer := time.NewTimer(timeout)
    defer timer.Stop()
    select {
    case <-done:
        return nil
    case <-timer.C:
        return fmt.Errorf("timed out waiting for Sudoku process after %s", timeout)
    }
}

type logWriter struct {
    mu sync.Mutex
    label string
    buf string
}

func (w *logWriter) Write(p []byte) (int, error) {
    w.mu.Lock()
    defer w.mu.Unlock()
    w.buf += string(p)
    for {
        idx := len(w.buf)
        for i, ch := range w.buf {
            if ch == '\n' {
                idx = i
                break
            }
        }
        if idx == len(w.buf) {
            break
        }
        line := w.buf[:idx]
        w.buf = w.buf[idx+1:]
        if line != "" {
            logger.Infof("sudoku: %s | %s", w.label, line)
        }
    }
    return len(p), nil
}

func configDir(binDir string) string {
    return filepath.Join(binDir, "sudoku")
}

func configPath(binDir string, inboundID int) string {
    return filepath.Join(configDir(binDir), fmt.Sprintf("sudoku-%d.json", inboundID))
}

func keyPath(binDir string, inboundID int) string {
    return filepath.Join(configDir(binDir), fmt.Sprintf("sudoku-%d.key", inboundID))
}
