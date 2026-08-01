//go:build windows

package syscallutil

import (
	"os"
	"syscall"
)

func KillProcess(pid int) error {
	return nil
}

func KillProcessGroup(pid int) error {
	return nil
}

func GetSysProcAttrSetpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}

func SetNonblock(f *os.File, nonblocking bool) error {
	return nil
}
