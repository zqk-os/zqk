//go:build windows

package syscallutil

import (
	"errors"
	"fmt"
	"os"
)

func errWouldBlock() error { return errors.New("would block") }

// SignalTerminate best-effort terminates pid on Windows.
func SignalTerminate(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}

// SignalExists probes whether pid is alive.
func SignalExists(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	// Windows FindProcess always succeeds for any pid; Signal(0) is not portable.
	// Use a no-op probe: attempt to open via Kill with signal 0 unsupported —
	// treat FindProcess success as inconclusive; prefer IsProcessAlive for callers.
	_ = proc
	return fmt.Errorf("signal 0 unsupported on windows")
}
