//go:build linux

package coremigration

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/SawaMEN/3x-ui/v3/internal/hiddify"
	"github.com/SawaMEN/3x-ui/v3/internal/singbox"
	"github.com/SawaMEN/3x-ui/v3/internal/xray"
)

func assertStopped() error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	configs := map[string]bool{}
	for _, p := range []string{xray.GetConfigPath(), singbox.GetConfigPath(), hiddify.GetConfigPath()} {
		a, err := filepath.Abs(p)
		if err != nil {
			return err
		}
		configs[a] = true
	}
	panel := filepath.Join(os.Getenv("XUI_MAIN_FOLDER"), "x-ui")
	if os.Getenv("XUI_MAIN_FOLDER") == "" {
		panel = "/usr/local/x-ui/x-ui"
	}
	exe, _ := os.Executable()
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid == os.Getpid() {
			continue
		}
		proc := filepath.Join("/proc", entry.Name())
		target, err := os.Readlink(filepath.Join(proc, "exe"))
		if err != nil {
			continue
		}
		target = strings.TrimSuffix(target, " (deleted)")
		if target == exe || target == panel {
			return fmt.Errorf("panel PID %d is still running; stop x-ui", pid)
		}
		args, err := os.ReadFile(filepath.Join(proc, "cmdline"))
		if err != nil {
			continue
		}
		words := strings.Split(string(args), "\x00")
		for i, word := range words {
			if (word == "-c" || word == "-config" || word == "--config") && i+1 < len(words) {
				candidate := words[i+1]
				if !filepath.IsAbs(candidate) {
					cwd, _ := os.Readlink(filepath.Join(proc, "cwd"))
					candidate = filepath.Join(cwd, candidate)
				}
				if !configs[filepath.Clean(candidate)] {
					continue
				}
				return fmt.Errorf("core PID %d still owns installation config; stop x-ui and its cores", pid)
			}
		}
	}
	return nil
}
