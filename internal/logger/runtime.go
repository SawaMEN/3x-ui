package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/config"
	"github.com/SawaMEN/3x-ui/v3/internal/util/tail"
)

// ErrorWriter records every diagnostic line, including stack frames, in both
// the persistent log and the UI buffer. It must not call the standard logger.
type ErrorWriter struct{}

func (ErrorWriter) Write(p []byte) (int, error) {
	for _, line := range strings.Split(strings.TrimRight(string(p), "\r\n"), "\n") {
		if line != "" {
			Error(strings.TrimSuffix(line, "\r"))
		}
	}
	return len(p), nil
}

// StandardLogger captures net/http errors that otherwise go only to stderr.
func StandardLogger() *log.Logger {
	return log.New(ErrorWriter{}, "HTTP: ", 0)
}

// InitRuntimeDiagnostics is called once after acquiring the panel process lease.
// The runtime duplicates the descriptor, so fatal panics in any goroutine survive
// process exit without depending on a recover handler or logger locks.
func InitRuntimeDiagnostics() error {
	log.SetOutput(ErrorWriter{})
	log.SetFlags(0)
	path := filepath.Join(config.GetLogFolder(), "panic.log")
	lines, err := tail.ReadTailLines(path, 0, 64<<10)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read previous runtime crash: %w", err)
	}
	if len(lines) > 0 {
		Error("Previous panel process crashed; runtime report follows (last 64 KiB):")
		for i := len(lines) - 1; i >= 0; i-- {
			Errorf("runtime: %s", lines[i])
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open runtime crash log: %w", err)
	}
	defer f.Close()
	return debug.SetCrashOutput(f, debug.CrashOptions{})
}
