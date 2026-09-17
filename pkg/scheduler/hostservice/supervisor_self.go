package hostservice

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/process"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

// refuseIfCallerDescendsFromUnit fails closed when this process is running
// under the live supervisor for e (parent-chain identity, not command text).
// launchctl kickstart -k / bootout would otherwise SIGTERM that ancestor.
func refuseIfCallerDescendsFromUnit(e Entry) error {
	pid := liveSupervisorPID(e.AbsRoot)
	if pid <= 0 {
		return nil
	}
	if !process.IsAncestorPID(pid, os.Getpid()) {
		return nil
	}
	return errfmt.Errorf(
		"refusing host-service mutation of %s: caller pid %d is a descendant of supervisor pid %d",
		e.UnitLabel, os.Getpid(), pid,
	)
}

func liveSupervisorPID(absRoot string) int {
	if strings.TrimSpace(absRoot) == "" {
		return 0
	}
	raw, err := fileutil.ReadFile(filepath.Join(absRoot, paths.ProjectDataDir, paths.SchedulerDir, paths.SchedulerPIDFile))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	if err != nil || pid <= 0 {
		return 0
	}
	return pid
}
