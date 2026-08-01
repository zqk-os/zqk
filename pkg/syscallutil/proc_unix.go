//go:build !windows

package syscallutil

import (
	"os"
	"syscall"
)

func KillProcess(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}

func KillProcessGroup(pid int) error {
	return syscall.Kill(-pid, syscall.SIGKILL)
}

func GetSysProcAttrSetpgid() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

func SetNonblock(f *os.File, nonblocking bool) error {
	return syscall.SetNonblock(int(f.Fd()), nonblocking)
}
