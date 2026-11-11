//go:build !windows

package mcp

import (
	"os/exec"
	"syscall"
)

func setCommandProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

func killProcessGroupNeg(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
