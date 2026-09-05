package monitors

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/lanceman/zqk/pkg/healthcheck"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/scheduler"
	fileutil "github.com/lanceman/zqk/pkg/utils/fileutil"
)

const (
	schedulerEventsID   = "scheduler_events"
	schedulerEventsName = "Scheduler events (failures and slow jobs)"
	summaryNoFailures   = "no failures or slow jobs"
	summaryFailures     = "failures present"
	summarySlowJobs     = "slow jobs present"
	detailsPathKey      = "path"
	detailsFailuresKey  = "failures"
	detailsSlowKey      = "slow"
	detailsJobsKey      = "jobs"
)

// SchedulerEventsMonitor checks scheduler-metrics-summary for failures and slow jobs.
type SchedulerEventsMonitor struct{}

// NewSchedulerEventsMonitor returns a monitor that reads scheduler-metrics-summary.json and reports ok/degraded/fail.
func NewSchedulerEventsMonitor() *SchedulerEventsMonitor {
	return &SchedulerEventsMonitor{}
}

// ID implements healthcheck.Monitor.
func (m *SchedulerEventsMonitor) ID() string { return schedulerEventsID }

// Name implements healthcheck.Monitor.
func (m *SchedulerEventsMonitor) Name() string { return schedulerEventsName }

// Run implements healthcheck.Monitor.
func (m *SchedulerEventsMonitor) Run(ctx context.Context, projectRoot string) (*healthcheck.Result, error) {
	summaryPath := scheduler.SummaryPath(projectRoot)
	data, err := fileutil.ReadFile(summaryPath)
	if err != nil {
		if fileutil.IsNotExist(err) {
			return &healthcheck.Result{
				Status:  statusDegraded,
				Summary: "no summary file (scheduler metrics uninitialized or scheduler inactive)",
				Details: map[string]any{detailsPathKey: summaryPath},
			}, nil
		}
		return nil, err
	}
	var summary scheduler.SchedulerMetricsSummary
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil, err
	}
	if summary.JobStats == nil {
		summary.JobStats = make(map[string]scheduler.JobExecutionStats)
	}
	threshold := float64(scheduler.SlowJobThresholdSec)
	var failures, slow int
	var jobDetails []map[string]any
	for jobID, st := range summary.JobStats {
		runs := st.Completed + st.Failed
		if runs == 0 {
			continue
		}
		avgSec := st.TotalDurationSec / float64(runs)
		hasFailures := st.Failed > 0
		slowJob := avgSec > threshold
		if hasFailures {
			failures++
		}
		if slowJob {
			slow++
		}
		if hasFailures || slowJob {
			jobDetails = append(jobDetails, map[string]any{
				"job_id":               jobID,
				"completed":            st.Completed,
				objects.FieldKeyFailed: st.Failed,
				"avg_sec":              avgSec,
				"slow":                 slowJob,
			})
		}
	}
	status := statusOK
	summaryStr := summaryNoFailures
	if failures > 0 {
		status = statusFail
		summaryStr = summaryFailures
	} else if slow > 0 {
		status = statusDegraded
		summaryStr = summarySlowJobs
	}
	if failures > 0 || slow > 0 {
		summaryStr = fmt.Sprintf("%s (failures=%d slow=%d)", summaryStr, failures, slow)
	}
	return &healthcheck.Result{
		Status:  status,
		Summary: summaryStr,
		Details: map[string]any{
			"window_end_iso":   summary.WindowEndISO,
			detailsFailuresKey: failures,
			detailsSlowKey:     slow,
			detailsJobsKey:     jobDetails,
		},
	}, nil
}

func init() {
	healthcheck.DefaultRegistry.Register(NewSchedulerEventsMonitor())
}
