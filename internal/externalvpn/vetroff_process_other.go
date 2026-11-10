//go:build !linux

package externalvpn

import "os/exec"

func prepareVetroffCommand(_ *exec.Cmd) {}
