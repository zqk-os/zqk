package scheduler

import (
	"path/filepath"
	"strings"
)

// detachedDaemonStartArgs returns child argv for a detached scheduler daemon start.
// Standalone zqk-scheduler omits the "scheduler" subcommand; monolithic zqk includes it.
func detachedDaemonStartArgs(daemonExe string) []string {
	if strings.Contains(filepath.Base(daemonExe), schedulerDaemonProcessName) {
		return []string{schedulerArgStart, schedulerArgForeground}
	}
	return []string{schedulerArgScheduler, schedulerArgStart, schedulerArgForeground}
}
