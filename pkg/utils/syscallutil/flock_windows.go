//go:build windows

package syscallutil

import (
	"errors"
	"os"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const errFlockUnsupportedWindows = "syscallutil: flock not supported on Windows"

// FileFlock is unsupported on Windows.
func FileFlock(_ *fileutil.File, _ int) error {
	return errors.New(errFlockUnsupportedWindows)
}

// IsProcessAlive checks whether the given PID represents an active running process.
func IsProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	return err == nil && proc != nil
}
