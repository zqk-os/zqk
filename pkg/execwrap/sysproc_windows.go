//go:build windows || plan9

package execwrap

import (
	"os/exec"
)

// SetProcessGroup configures process group attributes on Windows (no-op).
func SetProcessGroup(cmd *exec.Cmd) {
	// No-op or Windows-specific job configuration if needed
}

// KillProcessGroup terminates the process associated with cmd.
func KillProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil || cmd.Process.Pid <= 0 {
		return nil
	}
	return cmd.Process.Kill()
}
