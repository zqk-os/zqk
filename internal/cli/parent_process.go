// Package cli re-exports process helpers for parent detection and meaningful activity (see pkg/process).
package cli

import (
	"time"

	"github.com/lanceman/zqk/pkg/process"
)

// ParentProcessName returns the executable/command name of the process with the given PID.
func ParentProcessName(pid int) string { return process.ParentProcessName(pid) }

// IsParentZqk returns true if the parent process is the main zqk executable.
func IsParentZqk() bool { return process.IsParentZqk() }

// TouchMeaningfulActivity records that the current command did real work.
func TouchMeaningfulActivity() { process.TouchMeaningfulActivity() }

// GetLastMeaningfulActivity returns the time of last meaningful activity.
func GetLastMeaningfulActivity() time.Time { return process.GetLastMeaningfulActivity() }
