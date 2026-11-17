//go:build !linux

package protocolserver

import "os/exec"

func prepareCommand(_ *exec.Cmd) {}
