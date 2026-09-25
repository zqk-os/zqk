//go:build !windows && !plan9

package testkit

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

type ProcessInfo struct {
	PID     int
	PPID    int
	Command string
}

func findChildProcesses(parentPid int) ([]ProcessInfo, error) {
	cmd := exec.Command("ps", "-A", "-o", "pid,ppid,command")
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	lines := strings.Split(out.String(), "\n")
	var children []ProcessInfo
	myPid := os.Getpid()

	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 != nil || err2 != nil {
			continue
		}
		// Skip self
		if pid == myPid {
			continue
		}
		command := strings.Join(fields[2:], " ")
		// Skip transient ps invocation
		if strings.HasPrefix(command, "ps ") || strings.HasPrefix(command, "ps -A") {
			continue
		}

		if ppid == parentPid {
			children = append(children, ProcessInfo{
				PID:     pid,
				PPID:    ppid,
				Command: command,
			})
		}
	}
	return children, nil
}

func killProcessByPid(pid int) error {
	return syscall.Kill(pid, syscall.SIGKILL)
}
