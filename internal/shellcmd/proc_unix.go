//go:build !windows

package shellcmd

import (
	"os/exec"
	"syscall"
)

// setProcessGroup starts the shell in its own process group and makes
// cancellation kill the whole group, so a pipeline's children ("slow | grep")
// die with it instead of holding the output pipe open.
func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
