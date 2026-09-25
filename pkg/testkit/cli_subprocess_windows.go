//go:build windows || plan9

package testkit

import (
	"os/exec"
)

func setProcessGroup(cmd *exec.Cmd) {
	// OS-specific job object or group assignment if needed
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	return cmd.Process.Kill()
}
