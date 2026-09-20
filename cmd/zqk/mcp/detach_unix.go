//go:build !windows

package mcp

import (
	"os/exec"
	"syscall"
)

func setDetach(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
