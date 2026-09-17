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

// DisconnectTimeoutMonitor temporarily disconnects the command timeout monitor and idle watchdog
// for interactive sessions (such as pagers). It registers optional onExit callbacks and returns
// a reconnect function that must be called when the interactive session ends.
func DisconnectTimeoutMonitor(onExit ...func()) func() {
	return process.DisconnectTimeoutMonitor(onExit...)
}

// IsTimeoutMonitorDisconnected reports whether the timeout monitor is currently disconnected.
func IsTimeoutMonitorDisconnected() bool {
	return process.IsTimeoutMonitorDisconnected()
}

// RegisterPagerExitCallback registers a callback to be invoked when the pager/interactive session exits.
func RegisterPagerExitCallback(cb func()) {
	process.RegisterPagerExitCallback(cb)
}
