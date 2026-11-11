//go:build windows

package testrunner

import "syscall"

func killProcessGroup(pgid int, sig syscall.Signal) error {
	_ = pgid
	_ = sig
	return nil
}
