//go:build !linux

package olcrtc

import "os/exec"

func configureInstallerProcess(cmd *exec.Cmd) {}
