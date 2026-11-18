package singbox

import (
	"os"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/SawaMEN/3x-ui/v3/internal/logger"
)

// ConsoleLogPath is separate from structured logging: panics bypass that logger.
func (p *Process) ConsoleLogPath() string { return p.config + ".console.log" }

type runtimeOutput struct {
	processOutput
	file   *lumberjack.Logger
	lastDay atomic.Int64
	report sync.Once
}

func newRuntimeOutput(path string) *runtimeOutput {
	output := &runtimeOutput{file: &lumberjack.Logger{
		Filename: path, MaxSize: 10, MaxBackups: 128, MaxAge: 90,
	}}
	day := time.Now().UTC().Unix() / 86400
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		day = info.ModTime().UTC().Unix() / 86400
	}
	output.lastDay.Store(day)
	return output
}

// lumberjack.Close closes the file, but not its rotation worker. Reusing this
// writer avoids leaking a worker on every crash/restart. The lifecycle lock and
// cmd.Wait ensure no old pipe writer remains when its memory buffer is reset.
func (p *Process) runtimeConsole() *runtimeOutput {
	if p.consoleOutput == nil {
		p.consoleOutput = newRuntimeOutput(p.ConsoleLogPath())
	} else {
		p.consoleOutput.mu.Lock()
		p.consoleOutput.buf = p.consoleOutput.buf[:0]
		p.consoleOutput.head = 0
		p.consoleOutput.mu.Unlock()
		p.consoleOutput.report = sync.Once{}
	}
	return p.consoleOutput
}

func (w *runtimeOutput) Write(b []byte) (int, error) {
	// Rotate the console log on the first write of each UTC day, even if
	// it never hits its size limit. Retention then applies to every segment.
	day := time.Now().UTC().Unix() / 86400
	previous := w.lastDay.Load()
	if previous != day && w.lastDay.CompareAndSwap(previous, day) {
		if err := w.file.Rotate(); err != nil {
			logger.Warning("Core console log rotation failed:", logger.ErrorSummary(err))
		}
	}
	n, _ := w.processOutput.Write(b)
	if _, err := w.file.Write(b); err != nil {
		w.report.Do(func() { logger.Errorf("Core console log write failed: %s", logger.ErrorSummary(err)) })
	}
	// Always drain child pipes, including when storage is full.
	return n, nil
}
func (w *runtimeOutput) Close() error { return w.file.Close() }
