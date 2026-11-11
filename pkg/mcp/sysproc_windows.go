//go:build windows

package mcp

import (
	"os"
	"os/exec"
)

func setCommandProcessGroup(cmd *exec.Cmd) {}

func killProcessGroupNeg(pid int) {
	if p, err := os.FindProcess(pid); err == nil {
		_ = p.Kill()
	}
}
