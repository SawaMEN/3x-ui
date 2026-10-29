//go:build linux

package externalvpn

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestVetroffRuntimeStopsWhenPanelDies(t *testing.T) {
	role := os.Getenv("XUI_TEST_VETROFF_PROCESS_ROLE")
	if role == "runtime" {
		var signal int32
		_, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, 2, uintptr(unsafe.Pointer(&signal)), 0, 0, 0, 0)
		if errno != 0 || signal != int32(syscall.SIGTERM) {
			t.Fatalf("runtime has no kernel parent-death protection: signal=%d error=%v", signal, errno)
		}
		if err := os.WriteFile(os.Getenv("XUI_TEST_VETROFF_PARENT_PIDFILE"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(60 * time.Second)
		return
	}
	if role == "panel" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestVetroffRuntimeStopsWhenPanelDies$")
		cmd.Env = append(os.Environ(), "XUI_TEST_VETROFF_PROCESS_ROLE=runtime")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		prepareVetroffCommand(cmd)
		_ = cmd.Run()
		return
	}
	pidFile := filepath.Join(t.TempDir(), "runtime.pid")
	parent := exec.Command(os.Args[0], "-test.run=^TestVetroffRuntimeStopsWhenPanelDies$")
	parent.Env = append(os.Environ(), "XUI_TEST_VETROFF_PARENT_PIDFILE="+pidFile, "XUI_TEST_VETROFF_PROCESS_ROLE=panel")
	parent.Stdout, parent.Stderr = os.Stdout, os.Stderr
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = parent.Process.Kill(); _ = parent.Wait() })
	var pid int
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if raw, err := os.ReadFile(pidFile); err == nil {
			pid, _ = strconv.Atoi(string(raw))
			if pid > 0 {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	if pid == 0 {
		t.Fatal("helper did not start runtime")
	}
	child, err := os.FindProcess(pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = child.Kill(); _ = child.Release() })
	if err := parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = parent.Wait()
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if os.IsNotExist(err) {
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		fields := strings.Fields(string(raw)[strings.LastIndex(string(raw), ")")+1:])
		if len(fields) > 0 && (fields[0] == "Z" || fields[0] == "X") {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("orphaned runtime retained its executable after panel death")
}
