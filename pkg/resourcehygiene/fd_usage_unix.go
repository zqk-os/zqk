//go:build !windows

package resourcehygiene

import (
	"os"
	"runtime"
	"syscall"
)

// GetProcessFDUsage returns the number of open file descriptors for the current process
// and the soft limit configured for NOFILE.
func GetProcessFDUsage() (int, int, error) {
	fdDir := "/dev/fd"
	if runtime.GOOS == "linux" {
		fdDir = "/proc/self/fd"
	}
	f, err := os.Open(fdDir)
	if err != nil {
		return -1, -1, err
	}
	defer f.Close()

	names, err := f.Readdirnames(-1)
	if err != nil {
		return -1, -1, err
	}
	// Subtract 1 for the directory fd used by Open/Readdirnames itself
	openCount := len(names)
	if openCount > 0 {
		openCount--
	}

	var rLimit syscall.Rlimit
	var maxLimit int = -1
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rLimit); err == nil {
		maxLimit = int(rLimit.Cur)
	}
	return openCount, maxLimit, nil
}
