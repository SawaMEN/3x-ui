package singbox

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func processMatchesConfig(pid int, binary, config string) bool {
	if !processMatchesBinary(pid, binary) {
		return false
	}
	procDir := filepath.Join("/proc", strconv.Itoa(pid))
	command, err := os.ReadFile(filepath.Join(procDir, "cmdline"))
	if err != nil {
		return false
	}
	args := strings.Split(strings.TrimSuffix(string(command), "\x00"), "\x00")
	if len(args) < 3 || args[1] != "run" {
		return false
	}
	var configured string
	for i := 2; i < len(args); i++ {
		switch {
		case args[i] == "-c" || args[i] == "--config":
			if i+1 >= len(args) || configured != "" {
				return false
			}
			i++
			configured = args[i]
		case strings.HasPrefix(args[i], "--config="):
			if configured != "" {
				return false
			}
			configured = strings.TrimPrefix(args[i], "--config=")
		case strings.HasPrefix(args[i], "-c="):
			if configured != "" {
				return false
			}
			configured = strings.TrimPrefix(args[i], "-c=")
		}
	}
	if configured == "" {
		return false
	}
	if !filepath.IsAbs(configured) {
		cwd, err := os.Readlink(filepath.Join(procDir, "cwd"))
		if err != nil {
			return false
		}
		configured = filepath.Join(cwd, configured)
	}
	canonical := func(path string) string {
		path = filepath.Clean(resolvePath(path))
		if resolved, err := filepath.EvalSymlinks(path); err == nil {
			return filepath.Clean(resolved)
		}
		return path
	}
	return canonical(configured) == canonical(config)
}

func (p *Process) rememberAppliedConfig() {
	data, err := os.ReadFile(p.config)
	if err != nil {
		return
	}
	p.mu.Lock()
	p.appliedConfig = data
	p.mu.Unlock()
}

// AppliedConfig is a snapshot taken when this instance was started/adopted.
// Reading the editable disk file would describe a candidate, not the process.
func (p *Process) AppliedConfig() []byte {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return append([]byte(nil), p.appliedConfig...)
}
