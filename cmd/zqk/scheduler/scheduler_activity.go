package scheduler

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cli/bldr_cli_cmd_v1"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// NewActivityCmd creates the activity command
func NewActivityCmd() *cobra.Command {
	helpBuilder := clipkg.DynamicHelpBuilder(
		"Show recent job activity including started but not completed jobs",
		"Show recent scheduler job activity including:",
		"- Data freshness (oldest/newest loaded event) and busyness",
		"- Missed timer triggers (compact table: codes HF/FQ/ST/LR/M + legend, schedule column)",
		"- Jobs started but not completed (compact: run, REC/WCH/INV codes + legend, full stuck_assessment in JSON)",
		"- Timer jobs: cron field breakdown and link to cron format docs",
		"- One row per job with latest started/completed/failed (relative times)",
		"- Flat recent event list (relative times)",
		"",
		"This helps validate cache refreshes, spot stuck runs, and interpret missed slots in context.",
	).
		ExcludeCommonFlags()

	activityCmd := clipkg.ApplyBuilder(bldr_cli_cmd_v1.NewSchedulerActivityCommandBuilder(), &cobra.Command{Use: "activity"})
	helpBuilder.ApplyToCommand(activityCmd)
	cli.BindAsyncProgress(activityCmd, runActivity)
	cli.AddCommonFlags(activityCmd)

	// DNA may already define these; only add when builder is still a stub.
	ensureActivityFlags(activityCmd)

	return activityCmd
}

func ensureActivityFlags(cmd *cobra.Command) {
	f := cmd.Flags()
	if f.Lookup("limit") == nil {
		f.Int("limit", 20, "Maximum number of recent events to show")
	}
	if f.Lookup("job-id") == nil {
		f.String("job-id", "", "Filter by specific job ID (optional)")
	}
	if f.Lookup("bypass-cache") == nil {
		f.Bool("bypass-cache", false, "Bypass cache and query audit events directly (slower, but more complete for small datasets)")
	}
	if f.Lookup("watch") == nil {
		f.Duration("watch", 0, "If greater than zero, re-print activity after this interval until interrupted (e.g. 10s)")
	}
	if f.Lookup("since") == nil {
		f.String("since", "", "Only include events on or after this instant: RFC3339 timestamp, or a duration ago (e.g. 24h, 30m)")
	}
	if f.Lookup("stuck-recent-after") == nil {
		f.Duration("stuck-recent-after", 2*time.Minute, "For started-but-not-completed: runs shorter than this are labeled likely healthy in-flight")
	}
	if f.Lookup("stuck-stale-after") == nil {
		f.Duration("stuck-stale-after", 30*time.Minute, "For started-but-not-completed: runs longer than this are labeled investigate")
	}
	if f.Lookup("compact") == nil {
		f.Bool("compact", false, "Use narrower job ID column and table width")
	}
}

func runActivity(cmd *cobra.Command, args []string) error {
	ctx := cli.GetContext(cmd)
	if ctx == nil {
		return errfmt.Errorf("failed to get context")
	}
	watch := flagDuration(cmd, "watch", 0)
	for iter := 0; ; iter++ {
		if err := showJobActivity(ctx, cmd); err != nil {
			return err
		}
		if watch <= 0 {
			return nil
		}
		sep := fmt.Sprintf("\n--- refresh every %s (pass %d; Ctrl+C to stop) ---\n\n", watch, iter+1)
		if err := cli.WriteOutput(cmd, []byte(sep)); err != nil {
			return err
		}
		time.Sleep(watch)
	}
}

// jobActivityEvent represents a single job execution event
type jobActivityEvent struct {
	JobID     string
	EventType string
	CreatedAt string
	Success   bool
	Duration  string
	Error     string
	Outcome   string
	// Set only for stuck-job rows (started, no completion in loaded activity).
	StuckAssessmentCode string `json:"stuck_assessment_code,omitempty" yaml:"stuck_assessment_code,omitempty"`
	StuckAssessment     string `json:"stuck_assessment,omitempty" yaml:"stuck_assessment,omitempty"`
}

// showJobActivity shows recent job activity including started but not completed jobs
func showJobActivity(ctx *cli.Context, cmd *cobra.Command) error {
	actCtx, err := initializeActivityContext(ctx, cmd)
	if err != nil {
		return err
	}

	if err := loadJobData(actCtx); err != nil {
		return err
	}

	activityEvents, stuckJobs, lastRuns, fullEvents := buildActivityEvents(actCtx)
	refTime := time.Now().UTC()
	jobSchedules := buildJobScheduleMap(actCtx.JobResult)
	daemonDown := checkDaemonStatus(actCtx.ProjectRoot, actCtx.JobResult)
	// Use full event set for missed-trigger detection so last-run and missed-count are accurate (not affected by --limit)
	missedTriggers := detectMissedTriggers(jobSchedules, fullEvents, refTime)

	busyness := buildSchedulerBusyness(actCtx.ProjectRoot, stuckJobs)

	// Emit coordinator event for activity query
	operationID := fmt.Sprintf("scheduler_activity_%d", time.Now().UnixNano())
	emitSchedulerActivityEventViaCoordinator(
		pkgctx.NewSystemContext(),
		actCtx.ProjectRoot,
		actCtx.StorageProvider,
		operationID,
		len(activityEvents),
		len(stuckJobs),
		len(missedTriggers),
		daemonDown,
		actCtx.JobIDFilter,
	)

	poolCreationDeclinedCount := int64(0)
	var recentStarted, recentCompleted, recentConflict []string
	if sched := schedulerpkg.GetGlobalScheduler(); sched != nil {
		snap := sched.GetMetricsSnapshot()
		poolCreationDeclinedCount = snap.Pool.CreationDeclined
		recentStarted = snap.RecentExecutionStarted
		recentCompleted = snap.RecentExecutionCompleted
		recentConflict = snap.RecentConflictDetected
	}

	watchInterval := flagDuration(cmd, "watch", 0)

	// Format output (--format on command chain)
	switch cli.GetFormat(cmd) {
	case cli.FormatJSON, cli.FormatJSONL, cli.FormatYAML:
		return outputActivityStructured(activityEvents, stuckJobs, missedTriggers, daemonDown, poolCreationDeclinedCount, busyness, recentStarted, recentCompleted, recentConflict, fullEvents, refTime, actCtx, watchInterval, cmd)
	default:
		return outputActivityTable(activityEvents, stuckJobs, jobSchedules, missedTriggers, daemonDown, poolCreationDeclinedCount, busyness, recentStarted, recentCompleted, recentConflict, lastRuns, fullEvents, refTime, actCtx, cmd)
	}
}

// buildSchedulerBusyness builds busyness stats: executing, pending queue, completed success/failed.
func buildSchedulerBusyness(projectRoot string, stuckJobs []jobActivityEvent) SchedulerBusyness {
	// Fallback: started-but-not-completed from audit/activity (often lags or misses run_wrapper).
	b := SchedulerBusyness{Executing: len(stuckJobs)}

	// Pending: peek trigger queue (best-effort, no lock)
	if projectRoot != emptyValue {
		q := schedulerpkg.NewJobTriggerQueue(projectRoot)
		if pending, err := q.PeekTriggerRequests(); err == nil {
			b.PendingInQueue = len(pending)
		}
	}

	// Prefer live daemon conflict set (in-process) or persisted snapshot (CLI talking to daemon).
	// false Executing:0 while test bundles run.
	if sched := schedulerpkg.GetGlobalScheduler(); sched != nil {
		if ids := sched.GetRunningJobIDs(); len(ids) > 0 {
			b.Executing = len(ids)
		} else {
			b.Executing = 0
		}
	} else if ids, _, ok := schedulerpkg.LoadRunningJobsSnapshot(projectRoot); ok {
		b.Executing = len(ids)
	}

	// Completed/failed totals from activity cache (fast, no audit List)
	cache := schedulerpkg.GetGlobalActivityCache()
	_ = cache.LoadCache(projectRoot)
	entries := cache.GetAllEntries()
	for _, e := range entries {
		b.CompletedSuccess += e.TotalCompleted
		b.CompletedFailed += e.TotalFailed
	}
	return b
}

// SchedulerBusyness summarizes executing, queued, and completed job counts for the scheduler busyness view.
type SchedulerBusyness struct {
	Executing        int // Jobs currently running (from conflict manager when in-process, else from "started but not completed")
	PendingInQueue   int // Trigger queue (pending cross-process trigger requests)
	CompletedSuccess int // Total completed successfully (from activity cache when available)
	CompletedFailed  int // Total failed (from activity cache when available)
}

// missedTrigger represents a scheduled run time that was missed
type missedTrigger struct {
	JobID             string    `json:"job_id"`
	ExpectedAt        time.Time `json:"expected_at"`
	LastRun           time.Time `json:"last_run"`
	MissedCount       int       `json:"missed_count"`
	ScheduleExpr      string    `json:"schedule_expression,omitempty"`
	ApproxIntervalSec int64     `json:"approx_interval_seconds,omitempty"`                          // estimated seconds between timer fires
	AssessmentCode    string    `json:"assessment_code,omitempty" yaml:"assessment_code,omitempty"` // short key for CLI (HF, FQ, ST, LR, M)
	Assessment        string    `json:"assessment,omitempty" yaml:"assessment,omitempty"`           // human-readable severity / what to do
	ContextNote       string    `json:"context_note,omitempty"`                                     // how missed count is computed
}

// detectMissedTriggers identifies scheduled run times that were missed
func detectMissedTriggers(jobSchedules map[string]map[string]any, activityEvents []jobActivityEvent, now time.Time) []missedTrigger {
	var missed []missedTrigger

	lastCompletions := buildLastCompletionsMap(activityEvents)

	for jobID, job := range jobSchedules {
		if !shouldCheckJobForMissedTriggers(job) {
			continue
		}

		trigger := processJobForMissedTriggers(jobID, job, lastCompletions, activityEvents, now)
		if trigger != nil {
			missed = append(missed, *trigger)
		}
	}

	return missed
}

// recentJobOutcomes holds per-job outcome lists from metrics (which fired vs conflict) for CLI display.
type recentJobOutcomes struct {
	ExecutionStarted   []string `json:"execution_started" yaml:"execution_started"`
	ExecutionCompleted []string `json:"execution_completed" yaml:"execution_completed"`
	ConflictDetected   []string `json:"conflict_detected" yaml:"conflict_detected"`
}

// activityDataFreshness summarizes oldest/newest event timestamps in the loaded activity set.
type activityDataFreshness struct {
	OldestEventRFC3339 string `json:"oldest_event_rfc3339,omitempty"`
	NewestEventRFC3339 string `json:"newest_event_rfc3339,omitempty"`
	OldestAgeHuman     string `json:"oldest_age_human,omitempty"` // relative to query time
	NewestAgeHuman     string `json:"newest_age_human,omitempty"`
	Note               string `json:"note,omitempty"`
}

// jobActivitySummaryRow is one job per row: latest started / completed / failed with human-relative ages.
type jobActivitySummaryRow struct {
	JobID               string `json:"job_id"`
	StartedRFC3339      string `json:"started_rfc3339,omitempty"`
	CompletedRFC3339    string `json:"completed_rfc3339,omitempty"`
	FailedRFC3339       string `json:"failed_rfc3339,omitempty"`
	LastActivityRFC3339 string `json:"last_activity_rfc3339,omitempty"`
	StartedHuman        string `json:"started_age_human,omitempty"`
	CompletedHuman      string `json:"completed_age_human,omitempty"`
	FailedHuman         string `json:"failed_age_human,omitempty"`
	LastActivityHuman   string `json:"last_activity_age_human,omitempty"`
}

// activityDisplayOptions records CLI switches used for this activity run (JSON/YAML).
type activityDisplayOptions struct {
	SinceRFC3339     string `json:"since_rfc3339,omitempty" yaml:"since_rfc3339,omitempty"`
	Compact          bool   `json:"compact" yaml:"compact"`
	StuckRecentAfter string `json:"stuck_recent_after,omitempty" yaml:"stuck_recent_after,omitempty"`
	StuckStaleAfter  string `json:"stuck_stale_after,omitempty" yaml:"stuck_stale_after,omitempty"`
	WatchIntervalSec int64  `json:"watch_interval_seconds,omitempty" yaml:"watch_interval_seconds,omitempty"`
}

func buildActivityDisplayOptions(act *ActivityContext, watch time.Duration) *activityDisplayOptions {
	if act == nil {
		return nil
	}
	o := &activityDisplayOptions{
		Compact:          act.Compact,
		StuckRecentAfter: act.StuckRecentThreshold.String(),
		StuckStaleAfter:  act.StuckStaleThreshold.String(),
	}
	if act.Since != nil {
		o.SinceRFC3339 = zqktime.FormatRFC3339UTCPtr(act.Since)
	}
	if watch > 0 {
		o.WatchIntervalSec = int64(watch.Round(time.Second) / time.Second)
	}
	return o
}

// activityStructuredPayload is JSON/YAML output for scheduler activity (same shape as prior marshal + WriteOutput).
type activityStructuredPayload struct {
	Display                   *activityDisplayOptions `json:"display,omitempty" yaml:"display,omitempty"`
	Busyness                  SchedulerBusyness       `json:"busyness" yaml:"busyness"`
	DaemonDown                bool                    `json:"daemon_down" yaml:"daemon_down"`
	PoolCreationDeclinedCount int64                   `json:"pool_creation_declined_count" yaml:"pool_creation_declined_count"`
	ActivityData              *activityDataFreshness  `json:"activity_data_freshness,omitempty" yaml:"activity_data_freshness,omitempty"`
	RecentActivity            []jobActivityEvent      `json:"recent_activity" yaml:"recent_activity"`
	RecentJobsSummary         []jobActivitySummaryRow `json:"recent_jobs_summary,omitempty" yaml:"recent_jobs_summary,omitempty"`
	StuckJobs                 []jobActivityEvent      `json:"stuck_jobs" yaml:"stuck_jobs"`
	MissedTriggers            []missedTrigger         `json:"missed_triggers" yaml:"missed_triggers"`
	MetricsRecent             recentJobOutcomes       `json:"metrics_recent,omitempty" yaml:"metrics_recent,omitempty"`
}

func buildActivityStructuredPayload(events, stuckJobs []jobActivityEvent, missedTriggers []missedTrigger, daemonDown bool, poolCreationDeclinedCount int64, busyness SchedulerBusyness, recentStarted, recentCompleted, recentConflict []string, fullEvents []jobActivityEvent, refTime time.Time, actCtx *ActivityContext, watch time.Duration) activityStructuredPayload {
	fresh := buildActivityDataFreshness(fullEvents, refTime)
	limit := 20
	if actCtx != nil && actCtx.Limit > 0 {
		limit = actCtx.Limit
	}
	summary := buildJobActivitySummaries(fullEvents, limit, refTime)
	if summary == nil {
		summary = []jobActivitySummaryRow{}
	}
	return activityStructuredPayload{
		Display:                   buildActivityDisplayOptions(actCtx, watch),
		Busyness:                  busyness,
		DaemonDown:                daemonDown,
		PoolCreationDeclinedCount: poolCreationDeclinedCount,
		ActivityData:              fresh,
		RecentActivity:            events,
		RecentJobsSummary:         summary,
		StuckJobs:                 stuckJobs,
		MissedTriggers:            missedTriggers,
		MetricsRecent: recentJobOutcomes{
			ExecutionStarted:   recentStarted,
			ExecutionCompleted: recentCompleted,
			ConflictDetected:   recentConflict,
		},
	}
}

func outputActivityStructured(events, stuckJobs []jobActivityEvent, missedTriggers []missedTrigger, daemonDown bool, poolCreationDeclinedCount int64, busyness SchedulerBusyness, recentStarted, recentCompleted, recentConflict []string, fullEvents []jobActivityEvent, refTime time.Time, actCtx *ActivityContext, watch time.Duration, cmd *cobra.Command) error {
	payload := buildActivityStructuredPayload(events, stuckJobs, missedTriggers, daemonDown, poolCreationDeclinedCount, busyness, recentStarted, recentCompleted, recentConflict, fullEvents, refTime, actCtx, watch)
	return cli.FormatOutput(cmd, payload)
}

// outputActivityTable outputs job activity as a table
func outputActivityTable(events, stuckJobs []jobActivityEvent, jobSchedules map[string]map[string]any, missedTriggers []missedTrigger, daemonDown bool, poolCreationDeclinedCount int64, busyness SchedulerBusyness, recentStarted, recentCompleted, recentConflict []string, lastRuns map[string]string, fullEvents []jobActivityEvent, refTime time.Time, actCtx *ActivityContext, cmd *cobra.Command) error {
	if actCtx != nil && actCtx.Compact {
		activityOutputCompact = true
		defer func() { activityOutputCompact = false }()
	}
	var buf strings.Builder
	outputDaemonDownWarningToBuffer(&buf, daemonDown)
	outputBusynessToBuffer(&buf, busyness)
	outputActivityDataFreshnessToBuffer(&buf, fullEvents, refTime)
	if poolCreationDeclinedCount > 0 {
		fmt.Fprintf(&buf, "Pool creation declined (fallback used): %d\n", poolCreationDeclinedCount)
	}
	outputMetricsRecentToBuffer(&buf, recentStarted, recentCompleted, recentConflict)
	outputMissedTriggersTableToBuffer(&buf, missedTriggers, refTime)
	stuckRecent := 2 * time.Minute
	stuckStale := 30 * time.Minute
	if actCtx != nil {
		stuckRecent = actCtx.StuckRecentThreshold
		stuckStale = actCtx.StuckStaleThreshold
	}
	outputStuckJobsTableToBuffer(&buf, stuckJobs, refTime, stuckRecent, stuckStale)
	outputJobScheduleTableToBuffer(&buf, jobSchedules, lastRuns)
	maxJobs := 20
	if actCtx != nil && actCtx.Limit > 0 {
		maxJobs = actCtx.Limit
	}
	outputRecentJobSummaryTableToBuffer(&buf, fullEvents, refTime, maxJobs)
	outputRecentActivityTableToBuffer(&buf, events, refTime)
	return cli.WriteOutput(cmd, []byte(buf.String()))
}

// outputMetricsRecentToBuffer writes metrics recent job IDs (started/completed/conflict) for human table output.
func outputMetricsRecentToBuffer(buf *strings.Builder, started, completed, conflict []string) {
	if len(started) == 0 && len(completed) == 0 && len(conflict) == 0 {
		return
	}
	buf.WriteString("Metrics (recent):\n")
	if len(started) > 0 {
		fmt.Fprintf(buf, "  execution_started:   %s\n", strings.Join(started, ", "))
	}
	if len(completed) > 0 {
		fmt.Fprintf(buf, "  execution_completed: %s\n", strings.Join(completed, ", "))
	}
	if len(conflict) > 0 {
		fmt.Fprintf(buf, "  conflict_detected:   %s\n", strings.Join(conflict, ", "))
	}
}
