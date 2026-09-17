//go:build !windows

package syscallutil

import (
	"syscall"

	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// FileFlock calls syscall.Flock on an open *os.File. Wraps the uintptr→int conversion
// required by syscall.Flock (gosec G115) in one place.
func FileFlock(f *fileutil.File, how int) error {
	return syscall.Flock(int(f.Fd()), how) //nolint:gosec // G115: FD from *os.File is valid for flock on Unix
}
