package storage

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/storage/audit"
)

func (s *AuditAggregationService) compressEventIDs(events []map[string]any) []string {
	return audit.CompressEventIDs(PrefixAudit, SeparatorRange, events)
}

// compareAuditID compares two audit IDs numerically (AUD-123 vs AUD-456)
func (s *AuditAggregationService) compareAuditID(id1, id2 string) int {
	return audit.CompareID(id1, id2)
}

// extractAuditIDNumber extracts the numeric part from an audit ID (AUD-123 -> 123)
func (s *AuditAggregationService) extractAuditIDNumber(id string) int {
	return audit.ExtractIDNumber(id)
}

// isConsecutive checks if two audit IDs are consecutive (AUD-100 and AUD-101)
func (s *AuditAggregationService) isConsecutive(id1, id2 string) bool {
	return audit.IsConsecutive(id1, id2)
}

// createAggregationMetric creates the aggregation metric object asynchronously (non-blocking)
// If a metric already exists for this time window, it updates the existing one instead
func (s *AuditAggregationService) createAggregationMetric(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	metric map[string]any,
) (string, error) {
	// Execute synchronously
	metricID, err := s.createAggregationMetricSync(ctx, secCtx, metric)
	if err != nil {
		return "", err
	}
	return metricID, nil
}

// createAggregationMetricSync contains the synchronous logic for creating aggregation metrics
// This is called from createAggregationMetric which wraps it in a goroutine for non-blocking execution
func (s *AuditAggregationService) mergeAggregationMetrics(metric1, metric2 map[string]any) map[string]any {
	return audit.MergeMetrics(PrefixAudit, SeparatorRange, metric1, metric2)
}

// findExistingMetricByWindow finds an existing aggregation metric for a given time window
func (s *AuditAggregationService) findExistingMetricByWindow(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd string,
) (map[string]any, error) {
	// Query for metrics with matching time window
	storageCtx := pkgctx.GetStorageContext()
	objs, err := s.query().List(ctx, secCtx, storageCtx, audit.FirstMatch(
		MetricKindAuditAggregation,
		audit.ExactAggregationWindow(windowStart, windowEnd),
	))
	if err != nil {
		return nil, err
	}
	if existing := audit.FirstObject(objs); existing != nil {
		return existing, nil
	}

	return nil, errfmt.Errorf(NoteNoExistingMetric)
}

func findOverlappingMetric(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	q audit.ObjectQuery,
	source string,
	windowStart, windowEnd string,
) string {
	startTime, endTime, ok := ParseTimeWindowRFC3339(windowStart, windowEnd)
	if !ok {
		return ""
	}
	storageCtx := pkgctx.GetStorageContext()
	objs, err := q.List(ctx, secCtx, storageCtx, audit.EventsNewestFirst(
		MetricKindAuditAggregation,
		audit.SourceEq(source),
		10,
	))
	if err != nil || len(objs) == 0 {
		return ""
	}
	return audit.FirstOverlappingID(objs, startTime, endTime)
}

// findExistingMetricByOverlappingWindow finds an existing aggregation metric with overlapping time window
// This is a fallback when exact time window match fails
func (s *AuditAggregationService) findExistingMetricByOverlappingWindow(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd string,
) string {
	return findOverlappingMetric(ctx, secCtx, s.query(), ValueAuditAggregationJob, windowStart, windowEnd)
}

// markEventsAsAggregated marks audit events as aggregated by updating their status
func (s *AuditAggregationService) markEventsAsAggregated(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	eventIDs []string,
) (int, error) {
	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	// Expand ID ranges if needed (eventIDs might be compressed ranges like "AUD-1..AUD-960")
	expandedIDs := ExpandIDRanges(eventIDs)

	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	return s.statuses().UpdateStatus(ctx, secCtx, expandedIDs, ValueStatusArchived)
}

// ExpandIDRange expands an ID range like "AUD-1..AUD-165" into individual IDs

// ExpandIDRanges expands a list of ID ranges and individual IDs into a flat list of individual IDs

// QueryOldAggregatedEvents queries for all archived/aggregated events older than cutoff time

// Events marked as archived after aggregation

// Older than cutoff

// No limit - get all old aggregated events

// QueryOldAuditEventsByAge returns audit_event IDs older than cutoff (by created_at).
// Used for catch-up cleanup when aggregation has been failing and event count is high.
// limit caps the number of IDs returned per call (0 = no limit; use 1000-2000 for batching).
// Uses high-volume event cache for fast queries when available.

// Try high-volume event cache first (fast path)

// Use cache for fast query

// Filter out already-archived events (cache doesn't store status)

// All events from cache were archived - fall through to storage query

// Cache returned empty but is populated - may be stale, fall through to storage query

// Cache not populated - try to build it quickly (non-blocking, with timeout)
// This helps when cache wasn't built before aggregation started
// Use shorter timeout (5s) to avoid blocking cleanup too long

// Check again after potential build

// Only use cache if we got results (cache might be partial with 10k limit)
// If cache returns empty but is populated, fall through to storage query

// Filter out already-archived events (cache doesn't store status)

// All events from cache were archived - fall through to storage query

// Fallback to storage query (slower but always works)
// Add timeout to prevent hanging on large datasets (17k+ events)
// Use 30 seconds - if query takes longer, return empty and let next batch try

// Exclude already-archived events to avoid redundant updates

// If timeout, return empty slice (not error) so job can continue with next batch

// filterOutArchivedEvents filters out event IDs that are already archived
// This is needed when using the cache (which doesn't store status) to avoid redundant updates

// Read status for all IDs in bulk (more efficient than individual reads)
// Use a timeout to avoid hanging

// Query for these specific IDs and check their status

// Only return non-archived events

// Limit to number of IDs we're checking

// If query fails or times out, return empty to fall back to storage query
// This is safer than potentially updating archived events

// For other errors, log and return empty to be safe

// Extract IDs from filtered results

// CleanupAggregatedEvents deletes or archives audit events that have been aggregated
// Skips events that are already archived to avoid redundant WAL entries.

// Filter out already-archived events to avoid redundant updates
// This prevents WAL entries for events that are already archived

// All events are already archived - nothing to do

// Update only non-archived events to archived status

// Delete events using optimized bulk delete
// Expand ID ranges if needed (eventIDs might be compressed ranges like "AUD-1..AUD-960")

// Update high-volume event cache before deletion

// Use optimized bulk delete if available (FileObjectStorage)

// 20 workers for parallel deletion

// Invalidate high-volume cache so next Count() uses index (not stale cache)

// Fallback to standard BulkDelete for other storage types

// Invalidate high-volume cache so next Count() uses index (not stale cache)

// If no events were deleted, check if there were errors

// Return first error as an example

// AuditAggregationResult is the storage alias for audit.AggregationResult.
type AuditAggregationResult = audit.AggregationResult

// CreateAggregationMetricForTest exposes createAggregationMetric for tests.
func (s *AuditAggregationService) CreateAggregationMetricForTest(ctx context.Context, secCtx *pkgctx.SecurityContext, metric map[string]any) (string, error) {
	return s.createAggregationMetric(ctx, secCtx, metric)
}

// FindExistingMetricByWindowForTest exposes findExistingMetricByWindow for tests.
func (s *AuditAggregationService) FindExistingMetricByWindowForTest(ctx context.Context, secCtx *pkgctx.SecurityContext, windowStart, windowEnd string) (map[string]any, error) {
	return s.findExistingMetricByWindow(ctx, secCtx, windowStart, windowEnd)
}

// FindExistingMetricByOverlappingWindowForTest exposes findExistingMetricByOverlappingWindow for tests.
func (s *AuditAggregationService) FindExistingMetricByOverlappingWindowForTest(ctx context.Context, secCtx *pkgctx.SecurityContext, windowStart, windowEnd string) string {
	return s.findExistingMetricByOverlappingWindow(ctx, secCtx, windowStart, windowEnd)
}

// MarkEventsAsAggregatedForTest exposes markEventsAsAggregated for tests.
func (s *AuditAggregationService) MarkEventsAsAggregatedForTest(ctx context.Context, secCtx *pkgctx.SecurityContext, eventIDs []string) (int, error) {
	return s.markEventsAsAggregated(ctx, secCtx, eventIDs)
}
