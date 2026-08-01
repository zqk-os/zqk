package scheduler

import (
	"os"
	"path/filepath"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
)

// WriteNoAutoRestartFile creates .zqk/scheduler/no-auto-restart so ensure-scheduler-running.sh (cron)
// will not start the daemon until the user runs "zqk scheduler start" again.
// Called by "zqk scheduler stop".
func WriteNoAutoRestartFile(projectRoot string) error {
	if projectRoot == emptyValue {
		return nil
	}
	dir := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir)
	if err := os.MkdirAll(dir, paths.DirPerm750); err != nil {
		return errfmt.Newf("mkdir scheduler dir").Wrap(err)
	}
	path := filepath.Join(dir, paths.SchedulerNoAutoRestartFile)
	return os.WriteFile(path, []byte{}, paths.FilePerm600)
}

// RemoveNoAutoRestartFile removes .zqk/scheduler/no-auto-restart so auto-restart (e.g. cron) may start the daemon again.
// Called by "zqk scheduler start".
func RemoveNoAutoRestartFile(projectRoot string) error {
	if projectRoot == emptyValue {
		return nil
	}
	path := filepath.Join(projectRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.SchedulerNoAutoRestartFile)
	return os.Remove(path)
}
