//go:build !windows

package syscallutil

import (
	"errors"
	"os"
	"syscall"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// FileFlock calls syscall.Flock on an open *os.File. Wraps the uintptr→int conversion
// required by syscall.Flock (gosec G115) in one place.
func FileFlock(f *fileutil.File, how int) error {
	return syscall.Flock(int(f.Fd()), how) //nolint:gosec // G115: FD from *os.File is valid for flock on Unix
}

// IsProcessAlive checks whether the given PID represents an active running process.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	if err == nil || errors.Is(err, syscall.EPERM) {
		return true
	}
	return false
}
