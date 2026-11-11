//go:build windows

package scheduler

import (
	"os"
	"syscall"
)

type sysProcAttrSetter interface {
	SetSysProcAttr(attr *syscall.SysProcAttr)
}

func setRunWrapperProcessGroup(cmd sysProcAttrSetter) {
	_ = cmd
}

func killPID(pid int, sig syscall.Signal) error {
	_ = sig
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Kill()
}
