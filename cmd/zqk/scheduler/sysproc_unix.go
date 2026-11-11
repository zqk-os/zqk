//go:build !windows

package scheduler

import (
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

func setDaemonSetsid(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func signalUSR1(pid int) error {
	return syscall.Kill(pid, syscall.SIGUSR1)
}

func notifyUSR1(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGUSR1)
}

func isUSR1(sig os.Signal) bool { return sig == syscall.SIGUSR1 }
