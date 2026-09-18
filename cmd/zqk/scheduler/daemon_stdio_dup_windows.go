//go:build windows

package scheduler

import (
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

func dupToStdFD(from *fileutil.File, toFD int) error {
	// Scheduler stdio redirect uses Dup2 only on Unix; callers gate on runtime.GOOS.
	_ = from
	_ = toFD
	return nil
}
