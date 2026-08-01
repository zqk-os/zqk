//go:build !windows

package scheduler

import "syscall"

func killProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}
