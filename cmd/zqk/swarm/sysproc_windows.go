//go:build windows

package swarm

import "os/exec"

func setSwarmDaemonSetsid(cmd *exec.Cmd) {}
