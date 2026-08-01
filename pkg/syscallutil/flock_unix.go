//go:build !windows

package syscallutil

import (
	"os"
	"syscall"
)

const (
	LockExNb = syscall.LOCK_EX | syscall.LOCK_NB
	LockUn   = syscall.LOCK_UN
)

func FileFlock(f *os.File, how int) error {
	return syscall.Flock(int(f.Fd()), how)
}
