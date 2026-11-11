//go:build !windows

package testrunner

import "syscall"

func killProcessGroup(pgid int, sig syscall.Signal) error {
	return syscall.Kill(pgid, sig)
}
