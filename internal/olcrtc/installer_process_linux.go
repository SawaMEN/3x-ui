package olcrtc

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// Cancel the whole build group so a timeout cannot leave Go/compiler children
// running or hold installer stdout open forever. SIGTERM permits rollback.
func configureInstallerProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 15 * time.Second
}
