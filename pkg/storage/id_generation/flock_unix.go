//go:build !windows

package id_generation

import (
	"os"
	"golang.org/x/sys/unix"
)

func lockSequenceFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX)
}

func unlockSequenceFile(f *os.File) error {
	return unix.Flock(int(f.Fd()), unix.LOCK_UN)
}
