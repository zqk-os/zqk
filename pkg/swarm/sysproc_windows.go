//go:build windows

package swarm

import (
	"os"
	"os/exec"
)

func setMCPProcessGroup(cmd *exec.Cmd) {}

func killMCPProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return os.ErrProcessDone
	}
	return cmd.Process.Kill()
}
