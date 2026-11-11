//go:build !windows

package syscallutil

import "syscall"

func errWouldBlock() error { return syscall.EWOULDBLOCK }

// SignalTerminate sends SIGTERM to pid (unix).
func SignalTerminate(pid int) error {
	return syscall.Kill(pid, syscall.SIGTERM)
}

// SignalExists probes whether pid is alive (signal 0).
func SignalExists(pid int) error {
	return syscall.Kill(pid, 0)
}
