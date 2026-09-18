package scheduler

import (
	"path/filepath"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/paths"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
)

// WriteNoAutoRestartFile creates .zqk/scheduler/no-auto-restart so ensure-scheduler-running.sh (cron)
// will not start the daemon until the user runs "zqk scheduler start" again.
// Called by "zqk scheduler stop".
func WriteNoAutoRestartFile(projectRoot string) error {
	if projectRoot == emptyValue {
		return nil
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir)
	if err := fileutil.MkdirAll(dir, paths.DirPerm750); err != nil {
		return errfmt.Newf("mkdir scheduler dir").Wrap(err)
	}
	path := filepath.Join(dir, paths.SchedulerNoAutoRestartFile)
	return fileutil.WriteFile(path, []byte{}, paths.FilePerm600)
}

// RemoveNoAutoRestartFile removes .zqk/scheduler/no-auto-restart so auto-restart (e.g. cron) may start the daemon again.
// Called by "zqk scheduler start".
func RemoveNoAutoRestartFile(projectRoot string) error {
	if projectRoot == emptyValue {
		return nil
	}
	return fileutil.Remove(NoAutoRestartFilePath(projectRoot))
}

// NoAutoRestartFilePath returns the path to the no-auto-restart marker for a project root.
func NoAutoRestartFilePath(projectRoot string) string {
	return filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.SchedulerNoAutoRestartFile)
}

// IsNoAutoRestartSet reports whether the no-auto-restart marker exists. Supervisors (cron ensure /
// watchdog) must honor it by NOT restarting the daemon: it records a deliberate "zqk scheduler stop".
func IsNoAutoRestartSet(projectRoot string) bool {
	if projectRoot == emptyValue {
		return false
	}
	_, err := fileutil.Stat(NoAutoRestartFilePath(projectRoot))
	return err == nil
}
