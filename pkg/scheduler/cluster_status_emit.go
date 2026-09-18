package scheduler

import (
	"os"
	"time"

	"github.com/zqk-os/zqk/pkg/scheduler/clusterstatus"
	"github.com/zqk-os/zqk/pkg/scheduler/hostservice"
)

// emitClusterStatusAfterJob best-effort publishes terminal job phase to the local
// cluster status plane (API-gateway shaped). Failures are logged and ignored.
func (s *Scheduler) emitClusterStatusAfterJob(job *ScheduledJob, runErr error) {
	if s == nil || job == nil || s.projectRoot == emptyValue {
		return
	}
	phase := clusterstatus.PhaseSucceeded
	errMsg := ""
	if runErr != nil {
		phase = clusterstatus.PhaseFailed
		errMsg = runErr.Error()
	}
	rootID := hostservice.RootID(s.projectRoot)
	nodeID, _ := os.Hostname()
	bus := clusterstatus.NewBus(s.projectRoot, clusterstatus.DefaultTTL)
	if err := bus.Emit(clusterstatus.Event{
		NodeID:    nodeID,
		RootID:    rootID,
		JobID:     job.ID,
		Phase:     phase,
		Error:     errMsg,
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		SchedulerJobExecutionLog(s.logger).Warn("cluster status emit failed").
			WithError(err).
			JobID(job.ID).
			Log()
	}
}

// ClusterStatusDecide is the fail-closed dependent gate for peer job watches.
func ClusterStatusDecide(projectRoot string, w clusterstatus.Watch) clusterstatus.DependentDecision {
	return clusterstatus.NewBus(projectRoot, clusterstatus.DefaultTTL).Decide(w)
}
