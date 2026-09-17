package scheduler

import "github.com/lanceman/zqk/pkg/config"

// loadJobsPausedScheduleExemptIDs reads scheduler_maintenance_config.yaml (see ZQK_SCHEDULER_MAINTENANCE_CONFIG).
// Returns nil when projectRoot is empty, load fails, or the config omits jobs_paused_schedule_exempt_job_ids —
// callers fall back to [isJobsPausedScheduleExemptBuiltin].
func loadJobsPausedScheduleExemptIDs(projectRoot string) map[string]bool {
	if projectRoot == emptyValue {
		return nil
	}
	cfg, err := config.NewSchedulerMaintenanceLoader(projectRoot).Load()
	if err != nil {
		return nil
	}
	if len(cfg.JobsPausedScheduleExemptJobIDs) == 0 {
		return nil
	}
	out := make(map[string]bool, len(cfg.JobsPausedScheduleExemptJobIDs))
	for _, id := range cfg.JobsPausedScheduleExemptJobIDs {
		if id != emptyValue {
			out[id] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// isJobsPausedScheduleExempt is true when this job must keep timer/immediate scheduling under jobs_paused.
func (s *Scheduler) isJobsPausedScheduleExempt(jobID string) bool {
	if s != nil && s.jobsPausedScheduleExemptIDs != nil {
		return s.jobsPausedScheduleExemptIDs[jobID]
	}
	return isJobsPausedScheduleExemptBuiltin(jobID)
}
