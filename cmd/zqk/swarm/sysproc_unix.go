//go:build !windows

package swarm

import (
	"os/exec"
	"syscall"
)

func setSwarmDaemonSetsid(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
