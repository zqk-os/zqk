package scheduler

import (
	stdcontext "context"
	"fmt"
	"strings"
	"time"

	"github.com/zqk-os/zqk/internal/cli"
	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	schedulerpkg "github.com/zqk-os/zqk/pkg/scheduler"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
	"github.com/spf13/cobra"
)

// JobHistoryContext groups state for job history operations
type JobHistoryContext struct {
	Ctx             *cli.Context
	Cmd             *cobra.Command
	ProjectRoot     string
	StorageFactory  *storagepkg.StorageFactory
	StorageProvider storagepkg.ObjectStorageProvider
	SecCtx          *pkgctx.SecurityContext
	StorageCtx      *pkgctx.StorageContext
	JobIDFilter     string
	Limit           int
	Since           *time.Time
	Until           *time.Time
	BypassCache     bool
}

// initializeJobHistoryContext sets up the job history context
func initializeJobHistoryContext(ctx *cli.Context, cmd *cobra.Command) (*JobHistoryContext, error) {
	projectRoot := ""
	if ctx != nil && ctx.ProjectRoot != emptyValue {
		// Prefer explicit context (e.g. tests pass testRoot so they hit test storage, not real project)
		projectRoot = ctx.ProjectRoot
	}
	if projectRoot == emptyValue {
		projectRoot = cli.ResolveProjectRoot(".")
	}
	if projectRoot == emptyValue {
		return nil, errfmt.Errorf("project root not found")
	}

	// Use global storage cache so we don't create a new storage instance (avoids blocking init and extra load)
	cmdCtx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 60*time.Second)
	defer cancel()

	storageProvider, err := storagepkg.GetGlobalStorageProviderCache().GetOrCreate(cmdCtx, projectRoot)
	if err != nil {
		return nil, errfmt.Newf("failed to get storage").Wrap(err)
	}

	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := ctx.GetStorageContext()

	jobIDFilter, _ := cmd.Flags().GetString("job-id") //nolint:errcheck
	limit, _ := cmd.Flags().GetInt("limit")           //nolint:errcheck
	if limit <= 0 {
		limit = 5000 // Default limit
	}
	if limit > 50000 {
		limit = 50000 // Cap at 50k for safety
	}

	// Parse time range filters (supports RFC3339, relative durations, and natural language)
	var sinceTime, untilTime *time.Time
	if sinceStr, _ := cmd.Flags().GetString("since"); sinceStr != emptyValue { //nolint:errcheck
		if t := parseTimeString(sinceStr, true); t != nil {
			sinceTime = t
		}
	}
	if untilStr, _ := cmd.Flags().GetString("until"); untilStr != emptyValue { //nolint:errcheck
		if t := parseTimeString(untilStr, false); t != nil {
			untilTime = t
		}
	}

	bypassCache, _ := cmd.Flags().GetBool("bypass-cache") //nolint:errcheck
	// If time filters are specified, we must bypass cache (cache doesn't support time ranges)
	if sinceTime != nil || untilTime != nil {
		bypassCache = true
	}

	return &JobHistoryContext{
		Ctx:             ctx,
		Cmd:             cmd,
		ProjectRoot:     projectRoot,
		StorageFactory:  nil, // not used when using global cache
		StorageProvider: storageProvider,
		SecCtx:          secCtx,
		StorageCtx:      storageCtx,
		JobIDFilter:     jobIDFilter,
		Limit:           limit,
		Since:           sinceTime,
		Until:           untilTime,
		BypassCache:     bypassCache,
	}, nil
}

// queryAuditEvents queries audit events for scheduler jobs
// Filters by both target_kind and event_type to ensure we get scheduler job execution events
// Uses high-volume event cache when available for faster queries
func queryAuditEvents(jhc *JobHistoryContext) ([]map[string]any, error) {
	// Try high-volume event cache first (fast path) if no complex filters
	cache := storagepkg.GetGlobalHighVolumeEventCache()
	useCache := false
	var cacheEventIDs []string

	if cache.IsPopulatedForProject(jhc.ProjectRoot) {
		// Can use cache if:
		// 1. Only time filters (or no filters)
		// 2. No event_type or target_kind filters (cache doesn't filter by those)
		// Note: We'll filter by target_kind/event_type after getting IDs from cache
		if jhc.Since != nil || jhc.Until != nil {
			startTime := time.Time{}
			endTime := time.Now().UTC().Add(24 * time.Hour)
			if jhc.Since != nil {
				startTime = *jhc.Since
			}
			if jhc.Until != nil {
				endTime = *jhc.Until
			}
			cacheEventIDs = cache.QueryByTimeWindow(startTime, endTime, jhc.Limit)
			useCache = len(cacheEventIDs) > 0
		} else if jhc.JobIDFilter == emptyValue {
			// No filters - can use cache for recent events
			weekAgo := time.Now().UTC().Add(-7 * 24 * time.Hour)
			cacheEventIDs = cache.QueryByTimeWindow(weekAgo, time.Now().UTC().Add(24*time.Hour), jhc.Limit)
			useCache = len(cacheEventIDs) > 0
		}
	}

	// Filter by target_kind AND event_type to get scheduler job execution events
	// Event types and target kind are dynamically loaded from specs and cached
	eventTypes, err := getSchedulerJobEventTypes()
	if err != nil {
		// Non-fatal - continue with empty list (will return no events)
		eventTypes = []string{}
	}
	targetKind := getSchedulerJobKind()
	filters := map[string]any{
		getAuditEventTargetKindField(): targetKind,
		getAuditEventEventTypeField(): map[string]any{
			"$in": eventTypes,
		},
	}
	if jhc.JobIDFilter != emptyValue {
		filters[getAuditEventTargetIDField()] = jhc.JobIDFilter
	}

	// Add time range filters if specified
	if jhc.Since != nil || jhc.Until != nil {
		timeFilter := make(map[string]any)
		if jhc.Since != nil {
			timeFilter["$gte"] = jhc.Since.Format(time.RFC3339)
		}
		if jhc.Until != nil {
			timeFilter["$lte"] = jhc.Until.Format(time.RFC3339)
		}
		if len(timeFilter) > 0 {
			filters[getAuditEventCreatedAtField()] = timeFilter
		}
	}

	// Use context with timeout to prevent hanging (60 second timeout)
	ctx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 60*time.Second)
	defer cancel()

	// Show progress for large queries
	if jhc.Limit > 1000 {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		schedulerpkg.SLog(logger).Info("Querying audit events").
			Int("limit", jhc.Limit).
			Bool("using_cache", useCache).
			Log()
	}

	var auditResult *storagepkg.QueryResult
	if useCache {
		// Read events from cache IDs (faster than full List)
		objects := make([]map[string]any, 0, len(cacheEventIDs))
		for _, id := range cacheEventIDs {
			if obj, err := jhc.StorageProvider.Read(ctx, jhc.SecCtx, id); err == nil {
				objects = append(objects, obj)
			}
		}
		auditResult = &storagepkg.QueryResult{Objects: objects}
	} else {
		// Fallback to full List query
		limit := jhc.Limit
		var err error
		auditResult, err = jhc.StorageProvider.List(ctx, jhc.SecCtx, jhc.StorageCtx, storagepkg.ListFilter{
			Kind:    schedulerKindAuditEvent,
			Filters: filters,
			SortBy:  getAuditEventCreatedAtField(),
			SortAsc: false,
			Limit:   limit,
		})
		if err != nil {
			if ctx.Err() == stdcontext.DeadlineExceeded {
				return nil, errfmt.Errorf("query timed out after 300 seconds (try reducing --limit or using --since/--until to narrow the time range)")
			}
			return nil, errfmt.Newf("failed to query audit events").Wrap(err)
		}
	}

	// Show progress after query completes
	if jhc.Limit > 1000 {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		schedulerpkg.SLog(logger).Info("Found audit events, filtering").
			Int("count", len(auditResult.Objects)).
			Log()
	}

	// Filter results in code to ensure we only get scheduler job events
	// This handles cases where $in might not be fully supported or events have different structures
	filteredEvents := make([]map[string]any, 0, len(auditResult.Objects))
	schedulerEventTypes, err := getSchedulerJobEventTypesMap()
	if err != nil {
		// Non-fatal - continue with empty map (will filter out all events)
		schedulerEventTypes = make(map[string]bool)
	}
	for _, event := range auditResult.Objects {
		fields := ExtractAuditEventFields(event)

		// Verify target_kind and event_type match scheduler job events
		if fields.ShouldProcess(jhc.JobIDFilter) && schedulerEventTypes[fields.EventType] {
			filteredEvents = append(filteredEvents, event)
		}
	}

	// Emit debug event via coordinator if verbose mode is enabled
	if jhc.Ctx.Verbose {
		profile := jhc.Ctx.Profile
		if profile == emptyValue {
			profile = string(pkgctx.ProfileSystem)
		}
		emitSchedulerHistoryDebugEventViaCoordinator(
			pkgctx.NewSystemContext(),
			jhc.ProjectRoot,
			jhc.StorageProvider,
			"query_audit_events",
			len(filteredEvents),
			len(auditResult.Objects),
			profile,
		)
	}

	return filteredEvents, nil
}

// parseTimeString parses a time string with support for:
// - RFC3339 format: "2026-01-01T00:00:00Z"
// - Relative durations: "1h", "12h", "1d", "7d", "30d"
// - Natural language: "today", "yesterday", "this-week", "this-month"
// isSince indicates if this is for --since (true) or --until (false)
func parseTimeString(timeStr string, isSince bool) *time.Time {
	now := time.Now().UTC()
	timeStr = strings.ToLower(strings.TrimSpace(timeStr))

	// Try RFC3339 format first
	if t, err := time.Parse(time.RFC3339, timeStr); err == nil {
		return &t
	}

	// Try relative duration (e.g., "1h", "12h", "1d")
	if duration, err := time.ParseDuration(timeStr); err == nil {
		if isSince {
			// For --since, subtract duration from now
			t := now.Add(-duration)
			return &t
		} else {
			// For --until, add duration to now (future time)
			t := now.Add(duration)
			return &t
		}
	}

	// Try natural language shortcuts
	switch timeStr {
	case "today":
		// Start of today (00:00:00)
		t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		return &t
	case "yesterday":
		// Start of yesterday
		yesterday := now.AddDate(0, 0, -1)
		t := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)
		return &t
	case "this-week":
		// Start of current week (Monday 00:00:00)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday = 7
		}
		daysSinceMonday := weekday - 1
		t := now.AddDate(0, 0, -daysSinceMonday)
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return &t
	case "last-week":
		// Start of last week (Monday 00:00:00)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		daysSinceMonday := weekday - 1
		t := now.AddDate(0, 0, -daysSinceMonday-7)
		t = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		return &t
	case "this-month":
		// Start of current month
		t := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return &t
	case "last-month":
		// Start of last month
		lastMonth := now.AddDate(0, -1, 0)
		t := time.Date(lastMonth.Year(), lastMonth.Month(), 1, 0, 0, 0, 0, time.UTC)
		return &t
	case "this-year":
		// Start of current year
		t := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
		return &t
	case "now":
		// Current time
		return &now
	}

	// If we can't parse it, return nil (will be ignored)
	return nil
}

// aggregateJobStats aggregates execution stats by job ID from audit events
func aggregateJobStats(jhc *JobHistoryContext, events []map[string]any) map[string]*jobStats {
	statsByJob := make(map[string]*jobStats)

	for _, event := range events {
		fields := ExtractAuditEventFields(event)

		// Skip if not a scheduler job event or doesn't match filter
		if !fields.ShouldProcess(jhc.JobIDFilter) {
			continue
		}

		if fields.TargetID == emptyValue {
			continue
		}

		// Skip started events - we only count completed/failed events for stats
		// This ensures stats reflect actual job executions, not just starts
		if fields.IsStartedEvent() {
			continue
		}

		// Only process completion events (completed or failed)
		if !fields.IsCompletionEvent() {
			continue
		}

		stats := getOrCreateJobStats(statsByJob, fields.TargetID)
		updateJobStatsFromEvent(stats, event, fields)
	}

	return statsByJob
}

// aggregateJobStatsFromCache aggregates execution stats from the activity cache
// This is much faster than querying audit events, but doesn't support time range filters
func aggregateJobStatsFromCache(jhc *JobHistoryContext) map[string]*jobStats {
	statsByJob := make(map[string]*jobStats)

	// Load cache
	cache := schedulerpkg.GetGlobalActivityCache()
	if err := cache.LoadCache(jhc.ProjectRoot); err != nil {
		// Cache load failed, return empty stats
		return statsByJob
	}

	// Get cache entries
	var entries map[string]*schedulerpkg.ActivityCacheEntry
	if jhc.JobIDFilter != emptyValue {
		// Get specific job
		if entry, exists := cache.GetEntry(jhc.JobIDFilter); exists {
			entries = map[string]*schedulerpkg.ActivityCacheEntry{
				jhc.JobIDFilter: entry,
			}
		}
	} else {
		// Get all entries
		entries = cache.GetAllEntries()
	}

	// Convert cache entries to jobStats
	for jobID, entry := range entries {
		stats := getOrCreateJobStats(statsByJob, jobID)

		// Total runs = completed + failed (not started, since started doesn't mean completed)
		stats.TotalRuns = entry.TotalCompleted + entry.TotalFailed
		stats.SuccessCount = entry.TotalCompleted
		stats.FailCount = entry.TotalFailed

		// Determine last run time and outcome
		// Use the most recent of LastCompleted or LastFailed
		if !entry.LastCompleted.IsZero() && !entry.LastFailed.IsZero() {
			if entry.LastCompleted.After(entry.LastFailed) {
				stats.LastRunAt = entry.LastCompleted.Format(time.RFC3339)
				stats.LastOutcome = "Success"
			} else {
				stats.LastRunAt = entry.LastFailed.Format(time.RFC3339)
				stats.LastOutcome = "Failed"
			}
		} else if !entry.LastCompleted.IsZero() {
			stats.LastRunAt = entry.LastCompleted.Format(time.RFC3339)
			stats.LastOutcome = "Success"
		} else if !entry.LastFailed.IsZero() {
			stats.LastRunAt = entry.LastFailed.Format(time.RFC3339)
			stats.LastOutcome = "Failed"
		}
	}

	return statsByJob
}

// getOrCreateJobStats gets or creates job stats for a job ID
func getOrCreateJobStats(statsByJob map[string]*jobStats, jobID string) *jobStats {
	if _, exists := statsByJob[jobID]; !exists {
		statsByJob[jobID] = &jobStats{
			JobID: jobID,
		}
	}
	return statsByJob[jobID]
}

// updateJobStatsFromEvent updates job stats from an audit event
// Only counts completion events (completed/failed), not started events
func updateJobStatsFromEvent(stats *jobStats, event map[string]any, fields *AuditEventFields) {
	// Only count completion events (completed or failed), not started events
	// This ensures stats reflect actual job executions, not just starts
	if !fields.IsCompletionEvent() {
		return // Skip started events - they don't represent completed executions
	}

	stats.TotalRuns++
	if stats.LastRunAt == emptyValue || fields.CreatedAt > stats.LastRunAt {
		stats.LastRunAt = fields.CreatedAt
		stats.LastOutcome = determineOutcomeFromEventType(fields.EventType)
	}

	updateSuccessFailureCounts(stats, event, fields)
}

// determineOutcomeFromEventType determines outcome from event type
func determineOutcomeFromEventType(eventType string) string {
	if isSchedulerJobCompletedEventType(eventType) {
		return "Success"
	}
	if isSchedulerJobFailedEventType(eventType) {
		return "Failed"
	}
	return eventType
}

// updateSuccessFailureCounts updates success/failure counts from event
func updateSuccessFailureCounts(stats *jobStats, _ map[string]any, fields *AuditEventFields) {
	// Check metadata first (coordinator stores success in metadata)
	if fields.Metadata != nil {
		if success, ok := fields.Metadata[GetMetadataSuccessField()].(bool); ok {
			if success {
				stats.SuccessCount++
			} else {
				stats.FailCount++
			}
			return
		}
	}

	// Fallback: check event_type (for events created before coordinator integration)
	if fields.IsCompletedEvent() {
		stats.SuccessCount++
	} else if fields.IsFailedEvent() {
		stats.FailCount++
	}
	// Started events don't count as success or failure - they're in progress
	// Only count completed/failed events
}

// sortJobStatsList sorts job stats list by job ID
func sortJobStatsList(statsList []*jobStats) {
	for i := 0; i < len(statsList)-1; i++ {
		for j := i + 1; j < len(statsList); j++ {
			if statsList[i].JobID > statsList[j].JobID {
				statsList[i], statsList[j] = statsList[j], statsList[i]
			}
		}
	}
}

// outputJobHistory outputs job history in the requested format
// Delegates to shared OutputJobHistoryData component that uses coordinator
func outputJobHistory(jhc *JobHistoryContext, statsList []*jobStats) error {
	profile := jhc.Ctx.Profile
	if profile == emptyValue {
		profile = string(pkgctx.ProfileSystem)
	}

	// Create context with timeout for output operations
	outputCtx, cancel := stdcontext.WithTimeout(pkgctx.NewSystemContext(), 60*time.Second)
	defer cancel()

	return OutputJobHistoryData(
		outputCtx,
		jhc.Cmd,
		jhc.ProjectRoot,
		jhc.StorageProvider,
		statsList,
		jhc.JobIDFilter,
		profile,
	)
}

// handleEmptyJobHistory handles the case when no job history is found
func handleEmptyJobHistory(jhc *JobHistoryContext) error {
	var buf strings.Builder
	if jhc.JobIDFilter != emptyValue {
		fmt.Fprintf(&buf, "No execution history found for job %s.\n", jhc.JobIDFilter)
	} else {
		buf.WriteString("No execution history found for any scheduler jobs.\n")
	}
	buf.WriteString("Jobs will appear here after they have been executed at least once.\n")
	return cli.WriteOutput(jhc.Cmd, []byte(buf.String()))
}
