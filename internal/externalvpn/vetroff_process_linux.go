//go:build linux

package externalvpn

import (
	"os/exec"
	"syscall"
)

func prepareVetroffCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Pdeathsig: syscall.SIGTERM}
}
