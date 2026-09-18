//go:build windows

package syscallutil

import (
	"errors"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

const errFlockUnsupportedWindows = "syscallutil: flock not supported on Windows"

// FileFlock is unsupported on Windows.
func FileFlock(_ *fileutil.File, _ int) error {
	return errors.New(errFlockUnsupportedWindows)
}
