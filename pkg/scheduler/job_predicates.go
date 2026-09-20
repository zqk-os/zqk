package scheduler

import "github.com/zqk-os/zqk/pkg/objects"

// IsSchedulerJobMarkedForDeletion returns true when the raw scheduler_job object represents a one_time job
// that is marked for deletion (enabled=false or status=disabled). Such jobs are excluded from LoadJobs
// at init so they are never loaded or scheduled; the retention handler deletes them in the background.
// Single source of truth for the mark-for-delete predicate (DRY with job_loader and retention handler).
func IsSchedulerJobMarkedForDeletion(raw map[string]any) bool {
	if raw == nil {
		return false
	}
	execMode, _ := raw[objects.FieldKeyExecutionMode].(string)
	if execMode != ExecutionModeOneTime {
		return false
	}
	enabled, _ := raw[objects.FieldKeyEnabled].(bool)
	status, _ := raw[objects.FieldKeyStatus].(string)

	// Mark disabled jobs or finished one-time jobs for deletion.
	// This prevents the daemon from choking on thousands of completed test bundles.
	return !enabled || status == StatusDisabled || objects.GetGlobalStatusChecker().IsTerminal(objects.KindSchedulerJob, status)
}
