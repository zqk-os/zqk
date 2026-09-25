//go:build windows || plan9

package testkit

type ProcessInfo struct {
	PID     int
	PPID    int
	Command string
}

func findChildProcesses(parentPid int) ([]ProcessInfo, error) {
	return nil, nil
}

func killProcessByPid(pid int) error {
	return nil
}
