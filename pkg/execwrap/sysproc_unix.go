//go:build !windows && !plan9

package execwrap

import (
	"os/exec"
	"syscall"
)

// SetProcessGroup sets Setpgid: true on the command's SysProcAttr so the child runs in a new process group.
func SetProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// KillProcessGroup terminates the process group associated with cmd using SIGKILL.
func KillProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	// Negative pid sends signal to the entire process group
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
