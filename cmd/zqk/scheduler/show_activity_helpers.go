package scheduler

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	clipkg "github.com/zqk-os/zqk/pkg/cli"
	"github.com/zqk-os/zqk/pkg/cliapp"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// ActivityContext groups state for job activity display
type ActivityContext struct {
	ProjectRoot     string
	StorageProvider storagepkg.ObjectStorageProvider
	SecCtx          *pkgctx.SecurityContext
	StorageCtx      *pkgctx.StorageContext
	Limit           int
	JobIDFilter     string
	BypassCache     bool // If true, bypass cache and query audit events; if false, use cache (default)
	// QueryTimeout overrides the default 30s timeout for loading job/audit data (e.g. tests use 60s under parallel load)
	QueryTimeout time.Duration
	JobResult    *storagepkg.QueryResult
	AuditResult  *storagepkg.QueryResult
	// Since filters events to those at or after this time (UTC). Nil = no filter.
	Since *time.Time
	// StuckRecentThreshold / StuckStaleThreshold tune "started but not completed" labels (human table).
	StuckRecentThreshold time.Duration
	StuckStaleThreshold  time.Duration
	Compact              bool // Narrower columns for smaller terminals
}

// initializeActivityContext sets up the activity context
func initializeActivityContext(ctx *cli.Context, cmd *cobra.Command) (*ActivityContext, error) {
	if ctx == nil {
		return nil, errfmt.Errorf("cli context is required")
	}

	base, err := initSchedulerQueryBaseContext(ctx, 30*time.Second)
	if err != nil {
		return nil, err
	}

	limit, _ := cmd.Flags().GetInt("limit")               //nolint:errcheck
	jobIDFilter, _ := cmd.Flags().GetString("job-id")     //nolint:errcheck
	bypassCache, _ := cmd.Flags().GetBool("bypass-cache") //nolint:errcheck
	compact, _ := cmd.Flags().GetBool("compact")          //nolint:errcheck

	sinceStr, _ := cmd.Flags().GetString("since") //nolint:errcheck
	var sincePtr *time.Time
	if strings.TrimSpace(sinceStr) != emptyValue {
		sinceT, err := parseActivitySince(sinceStr, time.Now().UTC())
		if err != nil {
			return nil, err
		}
		sincePtr = &sinceT
	}

	stuckRecent := flagDuration(cmd, "stuck-recent-after", 2*time.Minute)
	if stuckRecent <= 0 {
		stuckRecent = 2 * time.Minute
	}
	stuckStale := flagDuration(cmd, "stuck-stale-after", 30*time.Minute)
	if stuckStale <= 0 {
		stuckStale = 30 * time.Minute
	}

	return &ActivityContext{
		ProjectRoot:          base.ProjectRoot,
		StorageProvider:      base.StorageProvider,
		SecCtx:               base.SecCtx,
		StorageCtx:           base.StorageCtx,
		Limit:                limit,
		JobIDFilter:          jobIDFilter,
		BypassCache:          bypassCache,
		Since:                sincePtr,
		StuckRecentThreshold: stuckRecent,
		StuckStaleThreshold:  stuckStale,
		Compact:              compact,
	}, nil
}

// parseActivitySince accepts RFC3339 or a duration subtracted from now (e.g. 24h, 30m).
func parseActivitySince(s string, now time.Time) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == emptyValue {
		return time.Time{}, errfmt.Errorf("empty since value")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), nil
	}
	if d, err := time.ParseDuration(s); err == nil {
		return now.Add(-d).UTC(), nil
	}
	return time.Time{}, errfmt.Errorf("invalid --since %q: use RFC3339 or a duration (e.g. 24h, 30m)", s)
}

// loadJobData loads scheduler jobs and audit events
func loadJobData(actCtx *ActivityContext) error {
	queryTimeout := actCtx.QueryTimeout
	if queryTimeout <= 0 {
		queryTimeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), queryTimeout)
	defer cancel()

	// Load scheduler jobs
	schedulerJobKind := getSchedulerJobKind()
	jobResult, err := actCtx.StorageProvider.List(ctx, actCtx.SecCtx, actCtx.StorageCtx, storagepkg.ListFilter{
		Kind: schedulerJobKind,
	})
	if err != nil {
		// Non-fatal - continue without job schedule info
		jobResult = &storagepkg.QueryResult{Objects: []map[string]any{}}
	}
	actCtx.JobResult = jobResult

	// Default: use activity cache (fast). Only query audit events if cache is bypassed
	// Cache is much faster for large datasets (30k+ events), but audit events may be faster for small datasets
	if !actCtx.BypassCache {
		// Load activity cache
		cache := schedulerpkg.GetGlobalActivityCache()
		if err := cache.LoadCache(actCtx.ProjectRoot); err != nil {
			// Cache load failed, use empty result
			actCtx.AuditResult = &storagepkg.QueryResult{Objects: []map[string]any{}}
			return nil
		}

		// Convert cache entries to audit event format for compatibility
		var cacheEvents []map[string]any
		if actCtx.JobIDFilter != emptyValue {
			// Get specific job
			if entry, exists := cache.GetEntry(actCtx.JobIDFilter); exists {
				cacheEvents = convertCacheEntryToAuditEvents(actCtx.JobIDFilter, entry)
			}
		} else {
			// Get all entries
			entries := cache.GetAllEntries()
			for jobID, entry := range entries {
				events := convertCacheEntryToAuditEvents(jobID, entry)
				cacheEvents = append(cacheEvents, events...)
			}
		}

		actCtx.AuditResult = &storagepkg.QueryResult{Objects: cacheEvents}
		return nil
	}

	// Query audit events
	// Filter by both target_kind and event_type to ensure we get scheduler job execution events
	// Event types and target kind are dynamically loaded from specs and cached
	eventTypes, err := getSchedulerJobEventTypes()
	if err != nil {
		// Non-fatal - use fallback values for test environments
		eventTypes = []string{
			schedulerJobStartedEvent,
			schedulerJobCompletedEvent,
			schedulerJobFailedEvent,
		}
	}
	targetKind := getSchedulerJobKind()

	// Build filters - only include event_type filter if we have event types
	filters := map[string]any{
		getAuditEventTargetKindField(): targetKind,
	}
	// Only add event_type filter if we have event types (empty $in might not work in all storage backends)
	if len(eventTypes) > 0 {
		filters[getAuditEventEventTypeField()] = map[string]any{
			"$in": eventTypes,
		}
	}
	if actCtx.JobIDFilter != emptyValue {
		filters[getAuditEventTargetIDField()] = actCtx.JobIDFilter
	}

	// Use a reasonable limit (default limit is 20, so 2x = 40, but use max to prevent issues)
	limit := actCtx.Limit * 2
	if limit == 0 || limit > 1000 {
		limit = 1000 // Reduced from 10k to 1k for performance (with 29k events, 10k is too slow)
	}

	// Add time filter to limit query to recent events (last 7 days)
	// This dramatically reduces the dataset from 29k+ to recent events only
	// Use $onOrAfter semantic operator for proper timestamp comparison
	now := time.Now().UTC()
	weekAgo := now.Add(-7 * 24 * time.Hour)
	timeFilter := map[string]any{
		"$onOrAfter": weekAgo.Format(time.RFC3339),
	}
	filters[getAuditEventCreatedAtField()] = timeFilter

	// Try high-volume event cache first (fast path)
	useCache, cacheEventIDs := queryCachedEventIDs(actCtx.ProjectRoot, weekAgo, now.Add(24*time.Hour), limit)

	var auditResult *storagepkg.QueryResult
	if useCache {
		// Read events from cache IDs (faster than full List)
		queryCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), queryTimeout)
		defer cancel()
		objects := make([]map[string]any, 0, len(cacheEventIDs))
		for _, id := range cacheEventIDs {
			if obj, err := actCtx.StorageProvider.Read(queryCtx, actCtx.SecCtx, id); err == nil {
				objects = append(objects, obj)
			}
		}
		auditResult = &storagepkg.QueryResult{Objects: objects}
	} else {
		// Fallback to full List query
		queryCtx, cancel := context.WithTimeout(pkgctx.NewSystemContext(), queryTimeout)
		defer cancel()

		var err error
		auditResult, err = actCtx.StorageProvider.List(queryCtx, actCtx.SecCtx, actCtx.StorageCtx, storagepkg.ListFilter{
			Kind:    schedulerKindAuditEvent,
			Filters: filters,
			SortBy:  getAuditEventCreatedAtField(),
			SortAsc: false, // Most recent first
			Limit:   limit,
		})
		if err != nil {
			return errfmt.Newf("failed to query audit events").Wrap(err)
		}
	}

	// Filter results in code to ensure we only get scheduler job events
	// This handles cases where $in might not be fully supported or events have different structures
	filteredEvents := make([]map[string]any, 0, len(auditResult.Objects))
	schedulerEventTypes := safeSchedulerJobEventTypesMap()
	schedulerJobKindForFilter := getSchedulerJobKind()
	for _, event := range auditResult.Objects {
		fields := ExtractAuditEventFields(event)

		// Verify target_kind matches scheduler job kind
		if fields.TargetKind != schedulerJobKindForFilter {
			continue
		}

		// If we have event types loaded, verify event_type matches
		// If event types map is empty (test environment), allow all scheduler job events through
		if len(schedulerEventTypes) > 0 && !schedulerEventTypes[fields.EventType] {
			continue
		}

		// Apply job ID filter if specified
		if actCtx.JobIDFilter == emptyValue || fields.TargetID == actCtx.JobIDFilter {
			filteredEvents = append(filteredEvents, event)
		}
	}

	// Update result with filtered events
	actCtx.AuditResult = &storagepkg.QueryResult{
		Objects: filteredEvents,
		Meta:    auditResult.Meta,
		Groups:  auditResult.Groups,
	}

	return nil
}

// convertCacheEntryToAuditEvents converts a cache entry to audit event format
// Returns the most recent events (started, completed, failed) for compatibility
func convertCacheEntryToAuditEvents(jobID string, entry *schedulerpkg.ActivityCacheEntry) []map[string]any {
	var events []map[string]any
	schedulerJobKind := getSchedulerJobKind()

	// Create events for the most recent activity of each type
	if !entry.LastStarted.IsZero() {
		events = append(events, map[string]any{
			getAuditEventEventTypeField():  schedulerJobStartedEvent,
			getAuditEventTargetKindField(): schedulerJobKind,
			getAuditEventTargetIDField():   jobID,
			getAuditEventCreatedAtField():  entry.LastStarted.Format(time.RFC3339),
			"job_id":                       jobID,
			"success":                      true,
			"event_time":                   entry.LastStarted,
		})
	}

	if !entry.LastCompleted.IsZero() {
		events = append(events, map[string]any{
			getAuditEventEventTypeField():   schedulerJobCompletedEvent,
			getAuditEventTargetKindField():  schedulerJobKind,
			getAuditEventTargetIDField():    jobID,
			getAuditEventCreatedAtField():   entry.LastCompleted.Format(time.RFC3339),
			"job_id":                        jobID,
			"success":                       true,
			objects.FieldKeyDurationSeconds: entry.LastDuration,
			"event_time":                    entry.LastCompleted,
		})
	}

	if !entry.LastFailed.IsZero() {
		events = append(events, map[string]any{
			getAuditEventEventTypeField():   schedulerJobFailedEvent,
			getAuditEventTargetKindField():  schedulerJobKind,
			getAuditEventTargetIDField():    jobID,
			getAuditEventCreatedAtField():   entry.LastFailed.Format(time.RFC3339),
			"job_id":                        jobID,
			"success":                       false,
			objects.FieldKeyDurationSeconds: entry.LastDuration,
			"error":                         entry.LastError,
			"event_time":                    entry.LastFailed,
		})
	}

	// Sort by event time (most recent first)
	// Simple sort: put most recent event first
	if len(events) > 0 {
		// Find most recent
		mostRecentIdx := 0
		mostRecentTime := time.Time{}
		for i, event := range events {
			if eventTime, ok := event["event_time"].(time.Time); ok {
				if eventTime.After(mostRecentTime) {
					mostRecentTime = eventTime
					mostRecentIdx = i
				}
			}
		}
		// Move most recent to front
		if mostRecentIdx > 0 {
			events[0], events[mostRecentIdx] = events[mostRecentIdx], events[0]
		}
	}

	return events
}

// buildActivityEvents builds activity events from audit results.
// It returns: (limited events for display, stuck jobs, last run time per job for Scheduled Jobs table, full events for missed-trigger detection).
// Last-run and missed-trigger data are derived from the full event set so they are accurate even when --limit truncates displayed recent activity.
func buildActivityEvents(actCtx *ActivityContext) ([]jobActivityEvent, []jobActivityEvent, map[string]string, []jobActivityEvent) {
	var activityEvents []jobActivityEvent
	startedJobs := make(map[string]jobActivityEvent) // job_id -> started event
	completedJobs := make(map[string]bool)           // job_id -> has completion
	lastRuns := make(map[string]string)              // job_id -> RFC3339 last completion time

	// Check if audit result is available
	if actCtx.AuditResult == nil || len(actCtx.AuditResult.Objects) == 0 {
		return nil, []jobActivityEvent{}, lastRuns, nil
	}

	schedulerJobKind := getSchedulerJobKind()

	// First pass: identify latest daemon start time
	var lastDaemonStart time.Time
	for _, event := range actCtx.AuditResult.Objects {
		fields := ExtractAuditEventFields(event)
		if strings.Contains(fields.EventType, "scheduler_start") {
			if t, ok := parseActivityEventTime(fields.CreatedAt); ok && t.After(lastDaemonStart) {
				lastDaemonStart = t
			}
		}
	}

	for _, event := range actCtx.AuditResult.Objects {
		fields := ExtractAuditEventFields(event)
		if fields.TargetKind != schedulerJobKind {
			continue
		}

		if fields.TargetID == emptyValue {
			continue
		}

		if actCtx.JobIDFilter != emptyValue && fields.TargetID != actCtx.JobIDFilter {
			continue
		}

		if actCtx.Since != nil {
			if et, ok := parseActivityEventTime(fields.CreatedAt); ok && et.Before(*actCtx.Since) {
				continue
			}
		}

		// Parse duration and error from metadata
		duration, errorMsg, success := parseEventMetadata(event)

		// Determine outcome and track completion
		outcome := determineEventOutcome(fields.EventType, fields.TargetID, fields.CreatedAt, startedJobs, completedJobs)

		activityEvents = append(activityEvents, jobActivityEvent{
			JobID:     fields.TargetID,
			EventType: fields.EventType,
			CreatedAt: fields.CreatedAt,
			Success:   success,
			Duration:  duration,
			Error:     errorMsg,
			Outcome:   outcome,
		})
	}

	// Build last-run map from full event set (before limit) so Scheduled Jobs and Missed Triggers show correct times
	for _, e := range activityEvents {
		if isSchedulerJobCompletionEventType(e.EventType) {
			if lastRuns[e.JobID] == emptyValue || e.CreatedAt > lastRuns[e.JobID] {
				lastRuns[e.JobID] = e.CreatedAt
			}
		}
	}

	// Find jobs that started but didn't complete; attach stuck assessment for table + JSON.
	now := time.Now().UTC()
	rec := actCtx.StuckRecentThreshold
	stale := actCtx.StuckStaleThreshold
	var stuckJobs []jobActivityEvent
	for jobID, startedEvent := range startedJobs {
		if !completedJobs[jobID] {
			ev := startedEvent
			if st, ok := parseActivityEventTime(ev.CreatedAt); ok {
				code, detail := stuckJobAssessmentParts(now.Sub(st), rec, stale)

				// If job started before current daemon run, it's a ghost job
				if !lastDaemonStart.IsZero() && st.Before(lastDaemonStart) {
					ev.StuckAssessmentCode = "GHOST"
					ev.StuckAssessment = "Ghost job from previous daemon run (never completed)"
					ev.Outcome = "Abandoned"
				} else {
					ev.StuckAssessmentCode = code
					ev.StuckAssessment = detail
				}
			}
			stuckJobs = append(stuckJobs, ev)
		}
	}

	// Limit only the events used for "Recent Job Activity" display; keep full list for missed-trigger detection
	fullEvents := activityEvents
	if actCtx.Limit > 0 && len(activityEvents) > actCtx.Limit {
		activityEvents = activityEvents[:actCtx.Limit]
	}

	return activityEvents, stuckJobs, lastRuns, fullEvents
}

// parseEventMetadata parses duration, error, and success from audit event maps.
// Storage-backed events nest these under metadata; the activity cache path uses top-level
// duration_seconds (see convertCacheEntryToAuditEvents). YAML/JSON can also represent
// numbers as int/int64/json.Number, so we coerce instead of requiring float64 only.
func parseEventMetadata(event map[string]any) (duration, errorMsg string, success bool) {
	metadata := metadataMapFromEvent(event)
	var successFromMetadata bool

	if metadata != nil {
		if d, ok := coerceFloat64(metadata[GetMetadataDurationField()]); ok {
			duration = fmt.Sprintf("%.2fs", d)
		}
		if errVal, ok := metadata[GetMetadataErrorField()]; ok {
			if v, ok := errVal.(string); ok {
				errorMsg = v
			}
		}
		if succVal, ok := metadata[GetMetadataSuccessField()]; ok {
			if v, ok := succVal.(bool); ok {
				success = v
				successFromMetadata = true
			}
		}
	}

	if duration == "" {
		if d, ok := coerceFloat64(event[objects.FieldKeyDurationSeconds]); ok {
			duration = fmt.Sprintf("%.2fs", d)
		} else if d, ok := coerceFloat64(event[GetMetadataDurationField()]); ok {
			duration = fmt.Sprintf("%.2fs", d)
		}
	}
	if errorMsg == "" {
		if errStr, ok := event[GetMetadataErrorField()].(string); ok {
			errorMsg = errStr
		}
	}
	if !successFromMetadata {
		if s, ok := event[GetMetadataSuccessField()].(bool); ok {
			success = s
		}
	}
	return duration, errorMsg, success
}

// metadataMapFromEvent returns nested metadata, including when YAML unmarshaling produced
// map[any]any so getMetadata's type assertion would fail.
func metadataMapFromEvent(event map[string]any) map[string]any {
	if m := getMetadata(event); m != nil {
		return m
	}
	metaKey := getAuditEventMetadataField()
	raw, ok := event[metaKey]
	if !ok || raw == nil {
		return nil
	}
	m, ok := coerceStringKeyMap(raw)
	if !ok {
		return nil
	}
	return m
}

func coerceStringKeyMap(v any) (map[string]any, bool) {
	if v == nil {
		return nil, false
	}
	switch v := v.(type) {
	case map[string]any:
		return v, true
	}

	// yaml.v2/v3 sometimes decodes nested maps as map[any]any
	if mAny, ok := v.(map[any]any); ok {
		mStr := make(map[string]any, len(mAny))
		for k, val := range mAny {
			if strK, ok := k.(string); ok {
				mStr[strK] = val
			} else {
				mStr[fmt.Sprintf("%v", k)] = val
			}
		}
		return mStr, true
	}

	return nil, false
}

func coerceFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

// determineEventOutcome determines the outcome of an event and tracks job state
func determineEventOutcome(eventType, targetID, createdAt string, startedJobs map[string]jobActivityEvent, completedJobs map[string]bool) string {
	outcome := eventType

	if isSchedulerJobCompletedEventType(eventType) {
		outcome = "Success"
		completedJobs[targetID] = true
	} else if isSchedulerJobFailedEventType(eventType) {
		outcome = "Failed"
		completedJobs[targetID] = true
	} else if isSchedulerJobStartedEventType(eventType) {
		outcome = "Started"
		// Track started jobs
		startedJobs[targetID] = jobActivityEvent{
			JobID:     targetID,
			EventType: eventType,
			CreatedAt: createdAt,
			Outcome:   "Started (not completed)",
		}
	}
	return outcome
}

// buildJobScheduleMap builds a map of job schedules
func buildJobScheduleMap(jobResult *storagepkg.QueryResult) map[string]map[string]any {
	jobSchedules := make(map[string]map[string]any)
	for _, job := range jobResult.Objects {
		jobID := getString(job, "id")
		if jobID != emptyValue {
			jobSchedules[jobID] = job
		}
	}
	return jobSchedules
}

// checkDaemonStatus checks if the daemon should be running but isn't.
// Uses the same "running" check as "scheduler status" (PID file + process) so activity
// and status agree; see pkg/scheduler/pid_file.go IsSchedulerRunning.
func checkDaemonStatus(projectRoot string, jobResult *storagepkg.QueryResult) bool {
	checkProjectRoot := projectRoot
	if checkProjectRoot == emptyValue {
		checkProjectRoot = cli.ResolveProjectRoot(".")
	}
	if checkProjectRoot == emptyValue {
		return false
	}

	// Same check as getSchedulerStatus: PID file exists and process is running
	running, _, err := schedulerpkg.IsSchedulerRunning(checkProjectRoot)
	if err != nil || running {
		return false
	}

	// Daemon not running; only show warning if there are enabled timer jobs that need it
	hasEnabledTimerJobs := false
	for _, job := range jobResult.Objects {
		triggerType := getString(job, "trigger_type")
		enabled := getBool(job, "enabled")
		scheduleExpr := getString(job, "schedule_expression")
		if triggerType == "timer" && enabled && scheduleExpr != emptyValue {
			hasEnabledTimerJobs = true
			break
		}
	}

	return hasEnabledTimerJobs
}

// outputDaemonDownWarningToBuffer outputs a warning if the daemon is down to a buffer
func outputDaemonDownWarningToBuffer(buf *strings.Builder, daemonDown bool) {
	if daemonDown {
		buf.WriteString("🚨 CRITICAL: Scheduler daemon is not running!\n")
		buf.WriteString("   The daemon should be running to execute scheduled jobs.\n")
		buf.WriteString(paths.RewriteCanonicalCLIInvocations("   Start it with: zqk scheduler start\n"))
		buf.WriteString("\n")
	}
}

// Uniform column widths and line length for scheduler activity tables (all sections aligned).
const (
	activityTableWidth   = 132
	activityColJobID     = 36 // Job ID (truncate long IDs)
	activityColDateTime  = 20 // "2006-01-02 15:04:05"
	activityColSchedule  = 22 // Cron expression
	activityColEnabled   = 8  // Yes/No
	activityColEventType = 28 // e.g. scheduler_job_completed
	activityColOutcome   = 14 // ✅ Success, ▶️ Started, ❌ Failed
	activityColDuration  = 10
	activityColHumanRel  = 22 // "2d ago", "3m ago"

	// Missed-triggers table only (dense layout; legend explains codes).
	missedColCount    = 4  // numeric missed count
	missedColInterval = 8  // ~10m
	missedColFirstUTC = 16 // 01-02 15:04 or 2006-01-02 15:04 when year ≠ ref
	missedColLastAge  = 10 // relative age
	missedColCode     = 3  // HF, M, …
	missedColSchedule = 22 // cron expression (truncated)

	// Stuck-jobs table (started, not completed): run duration + assessment code.
	stuckColRun   = 8  // 2d, 30m
	stuckColCode  = 3  // REC, WCH, INV
	stuckColStart = 16 // compact UTC (same rules as missed 1st)
)

// activityOutputCompact narrows job ID column and table width for --compact (set for duration of one render).
var activityOutputCompact bool

func effTableWidth() int {
	if activityOutputCompact {
		return 100
	}
	return activityTableWidth
}

func effColJobID() int {
	if activityOutputCompact {
		return 24
	}
	return activityColJobID
}

func effMissedColSchedule() int {
	if activityOutputCompact {
		return 16
	}
	return missedColSchedule
}

// outputBusynessToBuffer outputs the scheduler busyness summary (executing, queue, completed) to a buffer
func outputBusynessToBuffer(buf *strings.Builder, b SchedulerBusyness) {
	buf.WriteString("Scheduler busyness:\n")
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")
	fmt.Fprintf(buf, "  Executing:    %3d   (jobs currently running; daemon snapshot)\n", b.Executing)
	fmt.Fprintf(buf, "  In queue:     %3d   (pending trigger requests)\n", b.PendingInQueue)
	fmt.Fprintf(buf, "  Completed:    %3d   (success)\n", b.CompletedSuccess)
	fmt.Fprintf(buf, "  Failed:       %3d   (total failed)\n", b.CompletedFailed)
	buf.WriteString("\n")
}

// outputActivityDataFreshnessToBuffer prints oldest/newest event timestamps (staleness of loaded data).
func outputActivityDataFreshnessToBuffer(buf *strings.Builder, events []jobActivityEvent, refTime time.Time) {
	f := buildActivityDataFreshness(events, refTime)
	if f == nil {
		return
	}
	buf.WriteString("Activity data window (loaded events):\n")
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")
	if f.OldestEventRFC3339 != emptyValue {
		fmt.Fprintf(buf, "  Oldest event: %s  (%s)\n", f.OldestEventRFC3339, f.OldestAgeHuman)
	}
	if f.NewestEventRFC3339 != emptyValue {
		fmt.Fprintf(buf, "  Newest event: %s  (%s)\n", f.NewestEventRFC3339, f.NewestAgeHuman)
	}
	if f.Note != emptyValue {
		fmt.Fprintf(buf, "  %s\n", f.Note)
	}
	buf.WriteString("\n")
}

// buildActivityDataFreshness computes bounds for JSON/YAML and table output.
func buildActivityDataFreshness(events []jobActivityEvent, refTime time.Time) *activityDataFreshness {
	oldest, newest, ok := activityEventTimeBounds(events)
	if !ok {
		return &activityDataFreshness{
			Note: "No scheduler job events in the current load (cache empty or filter excluded everything).",
		}
	}
	return &activityDataFreshness{
		OldestEventRFC3339: zqktime.FormatRFC3339UTC(oldest),
		NewestEventRFC3339: zqktime.FormatRFC3339UTC(newest),
		OldestAgeHuman:     formatRelativeAge(oldest, refTime),
		NewestAgeHuman:     formatRelativeAge(newest, refTime),
		Note:               "Ages are relative to this command's snapshot time (not live-updating).",
	}
}

func activityEventTimeBounds(events []jobActivityEvent) (oldest, newest time.Time, ok bool) {
	for _, e := range events {
		t, okp := parseActivityEventTime(e.CreatedAt)
		if !okp {
			continue
		}
		if !ok {
			oldest, newest, ok = t, t, true
			continue
		}
		if t.Before(oldest) {
			oldest = t
		}
		if t.After(newest) {
			newest = t
		}
	}
	return oldest, newest, ok
}

func parseActivityEventTime(s string) (time.Time, bool) {
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC(), true
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z", s); err == nil {
		return t.UTC(), true
	}
	return time.Time{}, false
}

// formatRelativeAge renders "Xd ago" / "now" relative to ref (typically command start).
func formatRelativeAge(t, ref time.Time) string {
	if t.IsZero() {
		return "—"
	}
	d := ref.Sub(t)
	if d < time.Second {
		return "just now"
	}
	return formatDurationShort(d) + " ago"
}

// buildJobActivitySummaries aggregates latest started/completed/failed per job; keeps top maxJobs by last activity.
func buildJobActivitySummaries(events []jobActivityEvent, maxJobs int, refTime time.Time) []jobActivitySummaryRow {
	type agg struct {
		jobID     string
		started   *time.Time
		completed *time.Time
		failed    *time.Time
		last      time.Time
	}
	m := make(map[string]*agg)
	for _, ev := range events {
		t, ok := parseActivityEventTime(ev.CreatedAt)
		if !ok {
			continue
		}
		a, ok := m[ev.JobID]
		if !ok {
			a = &agg{jobID: ev.JobID}
			m[ev.JobID] = a
		}
		if t.After(a.last) {
			a.last = t
		}
		switch {
		case isSchedulerJobStartedEventType(ev.EventType):
			if a.started == nil || t.After(*a.started) {
				tt := t
				a.started = &tt
			}
		case isSchedulerJobCompletedEventType(ev.EventType):
			if a.completed == nil || t.After(*a.completed) {
				tt := t
				a.completed = &tt
			}
		case isSchedulerJobFailedEventType(ev.EventType):
			if a.failed == nil || t.After(*a.failed) {
				tt := t
				a.failed = &tt
			}
		}
	}
	type pair struct {
		last time.Time
		a    *agg
	}
	var pairs []pair
	for _, a := range m {
		pairs = append(pairs, pair{last: a.last, a: a})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].last.After(pairs[j].last) })
	if maxJobs > 0 && len(pairs) > maxJobs {
		pairs = pairs[:maxJobs]
	}
	ref := refTime
	if ref.IsZero() {
		ref = time.Now().UTC()
	}
	var rows []jobActivitySummaryRow
	for _, p := range pairs {
		a := p.a
		row := jobActivitySummaryRow{JobID: a.jobID}
		if a.started != nil {
			row.StartedRFC3339 = zqktime.FormatRFC3339UTCPtr(a.started)
			row.StartedHuman = formatRelativeAge(*a.started, ref)
		}
		if a.completed != nil {
			row.CompletedRFC3339 = zqktime.FormatRFC3339UTCPtr(a.completed)
			row.CompletedHuman = formatRelativeAge(*a.completed, ref)
		}
		if a.failed != nil {
			row.FailedRFC3339 = zqktime.FormatRFC3339UTCPtr(a.failed)
			row.FailedHuman = formatRelativeAge(*a.failed, ref)
		}
		row.LastActivityRFC3339 = zqktime.FormatRFC3339UTC(a.last)
		row.LastActivityHuman = formatRelativeAge(a.last, ref)
		rows = append(rows, row)
	}
	return rows
}

// outputRecentJobSummaryTableToBuffer prints one row per job with stage columns (relative times).
func outputRecentJobSummaryTableToBuffer(buf *strings.Builder, events []jobActivityEvent, refTime time.Time, maxJobs int) {
	if maxJobs <= 0 {
		maxJobs = 20
	}
	rows := buildJobActivitySummaries(events, maxJobs, refTime)
	if len(rows) == 0 {
		return
	}
	buf.WriteString("Recent jobs (one row per job; relative ages vs snapshot time):\n")
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")
	fmt.Fprintf(buf, "%-*s %-*s %-*s %-*s %-*s\n",
		effColJobID(), "Job ID",
		activityColHumanRel, "Started",
		activityColHumanRel, "Completed",
		activityColHumanRel, "Failed",
		activityColHumanRel, "Last activity")
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")
	for _, r := range rows {
		st := r.StartedHuman
		if st == emptyValue {
			st = "—"
		}
		co := r.CompletedHuman
		if co == emptyValue {
			co = "—"
		}
		fa := r.FailedHuman
		if fa == emptyValue {
			fa = "—"
		}
		la := r.LastActivityHuman
		if la == emptyValue {
			la = "—"
		}
		jid := clipkg.TruncateString(r.JobID, effColJobID())
		fmt.Fprintf(buf, "%-*s %-*s %-*s %-*s %-*s\n",
			effColJobID(), jid,
			activityColHumanRel, st,
			activityColHumanRel, co,
			activityColHumanRel, fa,
			activityColHumanRel, la)
	}
	buf.WriteString("\n")
}

// outputMissedTriggersLegendToBuffer prints code key and one-line window rule (paired with missed-triggers table).
func outputMissedTriggersLegendToBuffer(buf *strings.Builder) {
	buf.WriteString("  HF high-freq grace · FQ short cadence many gaps · ST first gap ≥48h · LR last run before miss · M default\n")
	buf.WriteString("  ~7d cron ticks vs completions in [tick,next); 1st=first missed UTC (year if ≠ snapshot); full RFC3339 in JSON/YAML.\n")
}

// outputMissedTriggersTableToBuffer outputs the missed triggers table to a buffer
func outputMissedTriggersTableToBuffer(buf *strings.Builder, missedTriggers []missedTrigger, refTime time.Time) {
	if len(missedTriggers) == 0 {
		return
	}

	buf.WriteString("⚠️  Missed timer triggers\n")
	outputMissedTriggersLegendToBuffer(buf)
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")
	fmt.Fprintf(buf, "%-*s %*s %-*s %-*s %-*s %-*s %-*s\n",
		effColJobID(), "Job ID",
		missedColCount, "n",
		missedColInterval, "~",
		missedColFirstUTC, "1st",
		missedColLastAge, "last",
		missedColCode, "cd",
		effMissedColSchedule(), "schedule")
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")
	schW := effMissedColSchedule()
	for _, mt := range missedTriggers {
		jobID := clipkg.TruncateString(mt.JobID, effColJobID())
		intervalStr := "—"
		if mt.ApproxIntervalSec > 0 {
			intervalStr = formatDurationShort(time.Duration(mt.ApproxIntervalSec) * time.Second)
		}
		intervalStr = clipkg.TruncateString(intervalStr, missedColInterval)
		firstUTC := clipkg.TruncateString(formatActivityCompactUTC(mt.ExpectedAt.UTC(), refTime), missedColFirstUTC)
		lastRunAge := "—"
		if !mt.LastRun.IsZero() {
			lastRunAge = clipkg.TruncateString(formatRelativeAge(mt.LastRun, refTime), missedColLastAge)
		}
		cd := strings.TrimSpace(mt.AssessmentCode)
		if cd == emptyValue {
			cd = "—"
		}
		sched := "—"
		if mt.ScheduleExpr != emptyValue {
			sched = clipkg.TruncateString(mt.ScheduleExpr, schW)
		}
		fmt.Fprintf(buf, "%-*s %*d %-*s %-*s %-*s %-*s %-*s\n",
			effColJobID(), jobID,
			missedColCount, mt.MissedCount,
			missedColInterval, intervalStr,
			missedColFirstUTC, firstUTC,
			missedColLastAge, lastRunAge,
			missedColCode, cd,
			schW, sched)
	}
	buf.WriteString("\n")
}

// outputStuckJobsLegendToBuffer explains stuck-job assessment codes (paired with stuck table).
func outputStuckJobsLegendToBuffer(buf *strings.Builder, stuckRecent, stuckStale time.Duration) {
	fmt.Fprintf(buf, "  REC within %s · WCH between %s and %s · INV past %s (see --stuck-* flags)\n",
		formatDurationShort(stuckRecent),
		formatDurationShort(stuckRecent),
		formatDurationShort(stuckStale),
		formatDurationShort(stuckStale))
	buf.WriteString("  run=time since start; cd=severity; full text in JSON stuck_assessment.\n")
}

// outputStuckJobsTableToBuffer outputs the stuck jobs table to a buffer
func outputStuckJobsTableToBuffer(buf *strings.Builder, stuckJobs []jobActivityEvent, refTime time.Time, stuckRecent, stuckStale time.Duration) {
	if len(stuckJobs) == 0 {
		return
	}

	buf.WriteString("⚠️  Jobs started but not completed\n")
	outputStuckJobsLegendToBuffer(buf, stuckRecent, stuckStale)
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")
	fmt.Fprintf(buf, "%-*s %-*s %-*s %-*s\n",
		effColJobID(), "Job ID",
		stuckColRun, "run",
		stuckColCode, "cd",
		stuckColStart, "started")
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")
	for _, job := range stuckJobs {
		jobID := clipkg.TruncateString(job.JobID, effColJobID())
		runFor := "—"
		cd := strings.TrimSpace(job.StuckAssessmentCode)
		if cd == emptyValue {
			cd = "—"
		}
		started := "—"
		if st, ok := parseActivityEventTime(job.CreatedAt); ok {
			d := refTime.Sub(st)
			runFor = clipkg.TruncateString(formatDurationShort(d), stuckColRun)
			started = clipkg.TruncateString(formatActivityCompactUTC(st.UTC(), refTime), stuckColStart)
		}
		fmt.Fprintf(buf, "%-*s %-*s %-*s %-*s\n",
			effColJobID(), jobID,
			stuckColRun, runFor,
			stuckColCode, cd,
			stuckColStart, started)
	}
	buf.WriteString("\n")
}

// Stuck-job assessment codes for dense CLI (see outputStuckJobsLegendToBuffer).
const (
	stuckAssessmentREC = "REC" // within stuck-recent-after
	stuckAssessmentWCH = "WCH" // between recent and stale threshold
	stuckAssessmentINV = "INV" // past stuck-stale-after
)

func stuckJobAssessmentParts(d, stuckRecent, stuckStale time.Duration) (code string, detail string) {
	switch {
	case d < stuckRecent:
		return stuckAssessmentREC, "likely healthy in-flight"
	case d < stuckStale:
		return stuckAssessmentWCH, "still running; check if duration is expected"
	default:
		return stuckAssessmentINV, "unusually long — investigate"
	}
}

// formatActivityCompactUTC prints MM-DD HH:MM in the snapshot year; adds year when t's year differs from ref.
func formatActivityCompactUTC(t, ref time.Time) string {
	t = t.UTC()
	ref = ref.UTC()
	if t.Year() != ref.Year() {
		return t.Format("2006-01-02 15:04")
	}
	return t.Format("01-02 15:04")
}

// outputJobScheduleTableToBuffer outputs the job schedule summary table to a buffer.
// lastRuns is job ID -> RFC3339 last completion time (from full event set, not truncated by --limit).
func outputJobScheduleTableToBuffer(buf *strings.Builder, jobSchedules map[string]map[string]any, lastRuns map[string]string) {
	if len(jobSchedules) == 0 {
		return
	}

	outputCronLegendToBuffer(buf)
	buf.WriteString("Scheduled Jobs (Timer-based):\n")
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")
	fmt.Fprintf(buf, "%-*s %-*s %-*s %-*s %-*s\n",
		effColJobID(), "Job ID",
		activityColSchedule, "Schedule",
		activityColDateTime, "Last Run",
		activityColDateTime, "Next Run",
		activityColEnabled, "Enabled")
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")

	if lastRuns == nil {
		lastRuns = make(map[string]string)
	}

	for jobID, job := range jobSchedules {
		if getString(job, "trigger_type") != "timer" {
			continue
		}

		schedule := getString(job, "schedule_expression")
		enabled := getBool(job, "enabled")
		enabledStr := "Yes"
		if !enabled {
			enabledStr = "No"
		}

		lastRun := formatLastRunTime(jobID, job, lastRuns)
		nextRun := formatNextRunTime(job)
		jobIDTrunc := clipkg.TruncateString(jobID, effColJobID())
		fmt.Fprintf(buf, "%-*s %-*s %-*s %-*s %-*s\n",
			effColJobID(), jobIDTrunc,
			activityColSchedule, clipkg.TruncateString(schedule, activityColSchedule),
			activityColDateTime, lastRun,
			activityColDateTime, nextRun,
			activityColEnabled, enabledStr)
		appendCronFieldsLine(buf, schedule)
	}
	buf.WriteString("\n")
}

func outputCronLegendToBuffer(buf *strings.Builder) {
	buf.WriteString("Cron schedule (timer jobs, robfig/cron v3):\n")
	buf.WriteString("  6-field:  second  minute  hour  day-of-month  month  day-of-week\n")
	buf.WriteString("  5-field:  minute  hour  dom  month  dow  (seconds implicitly 0)\n")
	buf.WriteString("  Full reference: https://pkg.go.dev/github.com/robfig/cron/v3\n")
	buf.WriteString("\n")
}

func appendCronFieldsLine(buf *strings.Builder, scheduleExpr string) {
	fields := strings.Fields(strings.TrimSpace(scheduleExpr))
	if len(fields) == 5 {
		fields = append([]string{"0"}, fields...)
	}
	if len(fields) != 6 {
		return
	}
	labels := []string{"sec", "min", "hour", "dom", "month", "dow"}
	var parts []string
	for i := range 6 {
		parts = append(parts, fmt.Sprintf("%s=%s", labels[i], fields[i]))
	}
	buf.WriteString("     ")
	buf.WriteString(strings.Join(parts, "  "))
	buf.WriteString("\n")
}

// formatLastRunTime formats the last run time for a job. Uses the later of activity-derived
// last run and the job's last_run_at from storage so that after a successful run (e.g. after
// daemon restart) the Scheduled Jobs table shows the same last run as used for missed-trigger logic.
func formatLastRunTime(jobID string, job map[string]any, lastRuns map[string]string) string {
	var best time.Time
	if lastRunStr := getString(job, "last_run_at"); lastRunStr != emptyValue {
		if t, err := time.Parse(time.RFC3339, lastRunStr); err == nil {
			best = t.UTC()
		}
		if best.IsZero() {
			if t, err := time.Parse("2006-01-02T15:04:05Z", lastRunStr); err == nil {
				best = t.UTC()
			}
		}
	}
	if lastRunTime, ok := lastRuns[jobID]; ok && lastRunTime != emptyValue {
		if t, err := time.Parse(time.RFC3339, lastRunTime); err == nil {
			tu := t.UTC()
			if tu.After(best) {
				best = tu
			}
		}
	}
	if best.IsZero() {
		return "Never"
	}
	return best.Format("2006-01-02 15:04:05")
}

// formatNextRunTime formats the next run time for a job. Zero/unknown times show as "N/A".
func formatNextRunTime(job map[string]any) string {
	nextRun := "N/A"
	if nextRunStr := getString(job, "next_run_at"); nextRunStr != emptyValue {
		if t, err := time.Parse(time.RFC3339, nextRunStr); err == nil && !t.IsZero() {
			nextRun = t.Format("2006-01-02 15:04:05")
		}
	}

	// Fallback: calculate from schedule_expression if N/A
	if nextRun == "N/A" {
		scheduleExpr := getString(job, "schedule_expression")
		if scheduleExpr != emptyValue {
			if schedule, err := parseCronSchedule(scheduleExpr); err == nil {
				next := schedule.Next(time.Now().UTC())
				if !next.IsZero() {
					nextRun = next.Format("2006-01-02 15:04:05")
				}
			}
		}
	}

	return nextRun
}

// outputRecentActivityTableToBuffer outputs the recent activity table to a buffer (flat event list).
func outputRecentActivityTableToBuffer(buf *strings.Builder, events []jobActivityEvent, refTime time.Time) {
	if len(events) == 0 {
		buf.WriteString("No recent job activity found.\n")
		return
	}

	buf.WriteString("Recent job events (flat list, newest first; time = relative to snapshot):\n")
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")
	fmt.Fprintf(buf, "%-*s %-*s %-*s %-*s %-*s\n",
		effColJobID(), "Job ID",
		activityColEventType, "Event Type",
		activityColOutcome, "Outcome",
		activityColDuration, "Duration",
		activityColHumanRel, "When (relative)")
	buf.WriteString(strings.Repeat("-", effTableWidth()) + "\n")

	for _, event := range events {
		timeStr := "—"
		if t, ok := parseActivityEventTime(event.CreatedAt); ok {
			timeStr = formatRelativeAge(t, refTime)
		}
		outcome := formatOutcomeWithEmoji(event.EventType, event.Outcome)
		duration := event.Duration
		if duration == emptyValue {
			duration = "-"
		}
		jobID := clipkg.TruncateString(event.JobID, effColJobID())
		eventType := clipkg.TruncateString(event.EventType, activityColEventType)
		fmt.Fprintf(buf, "%-*s %-*s %-*s %-*s %-*s\n",
			effColJobID(), jobID,
			activityColEventType, eventType,
			activityColOutcome, outcome,
			activityColDuration, duration,
			activityColHumanRel, timeStr)
	}
	buf.WriteString("\n")
	fmt.Fprintf(buf, "Total: %d recent event(s)\n", len(events))
}

// formatOutcomeWithEmoji formats outcome with emoji based on event type
func formatOutcomeWithEmoji(eventType, outcome string) string {
	if isSchedulerJobCompletedEventType(eventType) {
		return "✅ Success"
	}
	if isSchedulerJobFailedEventType(eventType) {
		return "❌ Failed"
	}
	if isSchedulerJobStartedEventType(eventType) {
		return "▶️  Started"
	}
	return outcome
}
