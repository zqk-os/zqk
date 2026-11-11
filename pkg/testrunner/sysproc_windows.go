//go:build windows

package testrunner

import "os/exec"

func setCommandProcessGroup(cmd *exec.Cmd) {
	// Process groups unsupported on Windows.
}
