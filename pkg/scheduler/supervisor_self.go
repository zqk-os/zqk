package scheduler

import (
	"os"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/process"
)

// ErrSupervisorSelfStop is returned when a caller would signal the scheduler
// process that is executing it (or an ancestor of it).
var ErrSupervisorSelfStop = errfmt.Errorf("refusing to signal the scheduler supervisor from one of its own descendants")

// RefuseSupervisorSelfStop fails closed when targetPID is this process or an
// ancestor. The check is the process tree, not the command text: any path that
// reaches SignalSchedulerByPID (CLI stop, --force, install helpers, host-service
// kickstart) is refused the same way.
func RefuseSupervisorSelfStop(targetPID int) error {
	if !process.IsAncestorPID(targetPID, os.Getpid()) {
		return nil
	}
	return errfmt.Newf("refusing to signal scheduler supervisor pid %d from descendant pid %d", targetPID, os.Getpid()).
		Wrap(ErrSupervisorSelfStop)
}
