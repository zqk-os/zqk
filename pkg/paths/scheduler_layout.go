package paths

import "path/filepath"

// SchedulerPIDFilePath returns the path to the scheduler daemon PID file for a project root:
// <projectRoot>/.zqk/scheduler/scheduler.pid — same layout as pkg/scheduler getPIDFilePath.
func SchedulerPIDFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, ProjectDataDir, SchedulerDir, SchedulerPIDFile)
}
