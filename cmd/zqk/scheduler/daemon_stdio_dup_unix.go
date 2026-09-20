//go:build !windows

package scheduler

import (
	"golang.org/x/sys/unix"

	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func dupToStdFD(from *fileutil.File, toFD int) error {
	return unix.Dup2(int(from.Fd()), toFD) //nolint:gosec // G115: FD from *os.File for Dup2 on Unix
}
