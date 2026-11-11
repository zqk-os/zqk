//go:build windows

package agent

import (
	"os"
	"os/exec"
)

func setAgentProcessGroup(cmd *exec.Cmd) {}

func killAgentProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Kill()
}
