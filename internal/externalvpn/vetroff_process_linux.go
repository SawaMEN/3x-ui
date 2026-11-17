//go:build linux

package externalvpn

import (
	"os/exec"
	"syscall"
)

func prepareVetroffCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM, Setpgid: true}
	if cmd.Cancel != nil {
		cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	}
}
