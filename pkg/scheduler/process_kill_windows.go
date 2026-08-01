//go:build windows

package scheduler

func killProcessGroup(pid int) error {
	return nil
}
