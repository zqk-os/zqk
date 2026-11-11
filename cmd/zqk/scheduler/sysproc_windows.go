//go:build windows

package scheduler

import (
	"fmt"
	"os"
	"os/exec"
)

func setDaemonSetsid(cmd *exec.Cmd) {}

func signalUSR1(pid int) error {
	return fmt.Errorf("SIGUSR1 unsupported on windows (pid=%d)", pid)
}

func notifyUSR1(ch chan<- os.Signal) {}

func isUSR1(sig os.Signal) bool { return false }
