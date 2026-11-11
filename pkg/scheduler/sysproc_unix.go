//go:build !windows

package scheduler

import "syscall"

type sysProcAttrSetter interface {
	SetSysProcAttr(attr *syscall.SysProcAttr)
}

func setRunWrapperProcessGroup(cmd sysProcAttrSetter) {
	cmd.SetSysProcAttr(&syscall.SysProcAttr{Setpgid: true})
}

func killPID(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}
