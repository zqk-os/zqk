//go:build windows

package mcp

import (
	"os/exec"
	"syscall"
)

func setDetach(cmd *exec.Cmd) {
	// CREATE_NEW_PROCESS_GROUP = 0x00000200
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200}
}
