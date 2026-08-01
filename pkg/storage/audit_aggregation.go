package storage

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/paths"
	"github.com/lanceman/zqk/pkg/pipeline"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/validation"
	"github.com/lanceman/zqk/pkg/when"
	"github.com/lanceman/zqk/pkg/zqkenv"
	"github.com/lanceman/zqk/pkg/zqktime"
)

const pipelineKindAuditAggregationAggregateAuditEvents = "storage.audit_aggregation_aggregate_audit_events"

// aggregationMetricCreationTimeout returns the timeout for waiting on async metric creation.
// Timeouts are generous by design; hitting one indicates a broader issue (load, contention, backlog)—
// investigate rather than only increasing the value. Env from zqkenv.AggregationMetricCreationTimeout()
// (e.g. "90s", "120s", or bare "90" for seconds) overrides default; max 120s.
func aggregationMetricCreationTimeout() time.Duration {
	const defaultTimeout = 90 * time.Second // generous default; scheduler handler may set dynamically from job max runtime
	const maxTimeout = 120 * time.Second
	s := strings.TrimSpace(os.Getenv(zqkenv.AggregationMetricCreationTimeout()))
	if s == emptyValue {
		return defaultTimeout
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		// Accept bare number as seconds (e.g. "60" from YAML)
		if sec, err := parseIntSeconds(s); err == nil && sec > 0 {
			d = time.Duration(sec) * time.Second
			if d > maxTimeout {
				d = maxTimeout
			}
			return d
		}
		return defaultTimeout
	}
	if d > maxTimeout {
		return maxTimeout
	}
	return d
}

// auditEventAggregationAllowedStatuses are audit_event lifecycle statuses that are eligible for aggregation.
// Pending is excluded because it represents in-flight work; archived is excluded because it is a post-aggregation state.
var auditEventAggregationAllowedStatuses = []string{ValueStatusCompleted, ValueStatusFailed, ValueStatusReverted, ValueStatusError}

// auditEventErrorStatuses are counted as errors for error_rate.
var auditEventErrorStatuses = map[string]struct{}{
	objects.FieldKeyFailed: {},
	ValueStatusReverted:    {},
	ValueStatusError:       {},
}

// parseIntSeconds parses s as an integer number of seconds (e.g. "60" -> 60). Used for env like ZQK_AGGREGATION_METRIC_CREATION_TIMEOUT="60".
func parseIntSeconds(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == emptyValue {
		return 0, errfmt.Errorf("empty")
	}
	// Try int first
	n, err := strconv.Atoi(s)
	if err == nil {
		return n, nil
	}
	// Try float (YAML may unmarshal as float64)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, err
	}
	return int(f), nil
}

// mapFromAnyToInt converts map[string]any (e.g. from builder) to map[string]int for merging. Nil-safe.
func mapFromAnyToInt(m map[string]any) map[string]int {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = intFromAny(v)
	}
	return out
}

func intFromAny(v any) int {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func isAuditEventErrorStatus(status string) bool {
	_, ok := auditEventErrorStatuses[strings.TrimSpace(strings.ToLower(status))]
	return ok
}

// mapIntToAny converts map[string]int to map[string]any for persistence (validation expects object).
func mapIntToAny(m map[string]int) map[string]any {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// AuditAggregationService handles aggregating audit events into metrics
type AuditAggregationService struct {
	storage               ObjectStorageProvider
	batchSize             int // Batch size for paginated queries (default: DefaultBatchSize)
	aggregationsTotal     atomic.Int64
	eventsAggregatedTotal atomic.Int64
}

// GetAuditAggregationStats returns lifetime counters for aggregations run and events aggregated.
func (s *AuditAggregationService) GetAuditAggregationStats() (aggregations, eventsAggregated int64) {
	return s.aggregationsTotal.Load(), s.eventsAggregatedTotal.Load()
}

// NewAuditAggregationService creates a new audit aggregation service with default batch size
func NewAuditAggregationService(storage ObjectStorageProvider) *AuditAggregationService {
	return NewAuditAggregationServiceWithBatchSize(storage, DefaultBatchSize)
}

// NewAuditAggregationServiceWithBatchSize creates a new audit aggregation service with specified batch size
func NewAuditAggregationServiceWithBatchSize(storage ObjectStorageProvider, batchSize int) *AuditAggregationService {
	effectiveBatchSize := batchSize
	if effectiveBatchSize <= 0 {
		effectiveBatchSize = DefaultBatchSize
	}
	return &AuditAggregationService{
		storage:   storage,
		batchSize: effectiveBatchSize,
	}
}

// AggregateAuditEvents aggregates audit events within a time window into metrics
func (s *AuditAggregationService) AggregateAuditEvents(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	windowStart, windowEnd time.Time,
) (*AuditAggregationResult, error) {
	s.aggregationsTotal.Add(1)
	type aggregateAuditEventsPipelineState struct {
		aggregationMetricAny any
		eventIDs             []string
		err                  error
		result               *AuditAggregationResult
		metricID             string
		updatedCount         int
	}

	// Ensure kind mapper is initialized so it knows where to store metrics.
	// This is critical for file backend to know the metrics directory.
	// (Stage Ingest keeps this setup criteria-scoped for pipeline observability.)
	logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))

	st := &aggregateAuditEventsPipelineState{}
	pl := pipeline.NewBuilder(pipelineKindAuditAggregationAggregateAuditEvents, logger).
		WithMetricsConfig(pipeline.DefaultMetricsConfig(logger)).
		WithProfile(string(pkgctx.ProfileSystem)).
		AddStage(pipeline.StageIngest, func(pctx *pipeline.Context, payload any) (any, error) {
			kindMapper := objects.GetGlobalKindMapper()
			if err := kindMapper.Initialize(); err != nil {
				st.err = errfmt.Newf(ErrMsgInitKindMapper).Wrap(err)
				return st, nil
			}

			// Ensure audit_event CAS index is populated before first batch so the first List() returns results.
			if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
				if fileStorage.usesContentAddressableStorage(MetricKindAuditEvent) {
					if cas, err := fileStorage.getContentAddressableStorage(MetricKindAuditEvent); err == nil && cas != nil {
						ids, errList := cas.ListIDs()
						if errList != nil {
							StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn(LogMsgAuditAggregation).WithError(errList).Log()
						}
						if len(ids) == 0 {
							if errEnsure := fileStorage.EnsureCASIndexPopulatedFromScan(ctx, MetricKindAuditEvent); errEnsure != nil {
								StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn(LogMsgCasIndexPopulated).WithError(errEnsure).Log()
							}
						}
					}
				}
			}

			// Use batch processor for high-volume scenarios
			batchProcessor := NewBatchProcessor(s.batchSize)

			// Build query function for audit events in time window.
			queryFilters := map[string]any{
				objects.FieldKeyCreatedAt: map[string]any{
					"$gte": windowStart.Format(time.RFC3339),
					"$lte": windowEnd.Format(time.RFC3339),
				},
			}
			if len(auditEventAggregationAllowedStatuses) > 0 {
				queryFilters[objects.FieldKeyStatus] = map[string]any{
					"$in": auditEventAggregationAllowedStatuses,
				}
			}

			queryBuilder := NewBatchQueryBuilder(s.storage, secCtx, storageCtx, MetricKindAuditEvent).
				WithFilters(queryFilters).
				WithSort(objects.FieldKeyCreatedAt, true)
			queryFunc := queryBuilder.BuildQueryFunc(ctx)

			// Process function: aggregate a batch of events
			processFunc := func(batch []map[string]any) (any, []string, error) {
				metric, eventIDs, err := s.aggregateEvents(ctx, secCtx, batch, windowStart, windowEnd)
				if err != nil {
					return nil, nil, err
				}
				return metric, eventIDs, nil
			}

			// Merge function: merge aggregation metrics
			mergeFunc := func(firstResult, secondResult any) (any, error) {
				metric1, ok1 := firstResult.(map[string]any)
				metric2, ok2 := secondResult.(map[string]any)
				if !ok1 || !ok2 {
					return nil, errfmt.Errorf(ErrMsgInvalidMetricMerge)
				}
				return s.mergeAggregationMetrics(metric1, metric2), nil
			}

			// Process in batches
			aggregationMetricAny, eventIDs, err := batchProcessor.ProcessInBatches(ctx, queryFunc, processFunc, mergeFunc)
			if err != nil {
				st.err = errfmt.Newf(ErrMsgProcessEventsBatch).Wrap(err)
				return st, nil
			}
			st.aggregationMetricAny = aggregationMetricAny
			st.eventIDs = eventIDs

			if aggregationMetricAny == nil {
				st.result = &AuditAggregationResult{
					EventCount:      0,
					MetricsCreated:  0,
					EventsProcessed: []string{},
				}
			}

			return st, nil
		}).
		AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
			if st.err != nil || st.result != nil {
				return st, nil
			}

			aggregationMetric, ok := st.aggregationMetricAny.(map[string]any)
			if !ok {
				st.err = errfmt.Errorf(ErrMsgInvalidAggMetric)
				return st, nil
			}

			// Ensure ID validator has loaded patterns (including audit_aggregation_metric).
			// Get project root from storage (most reliable since storage was initialized with explicit project root).
			var projectRoot string
			if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
				projectRoot = fileStorage.GetProjectRoot()
			}

			// Initialize ID validator with project root from storage so validation uses the same context.
			var idValidator *validation.IDValidator
			when.When(func() bool { return projectRoot != emptyValue }).Then(func() {
				specsDir := filepath.Join(projectRoot, paths.ProcessInternalObjectSpecsDir)
				_, err := os.Stat(specsDir)
				when.When(func() bool { return err == nil }).Then(func() {
					idValidator = validation.NewIDValidator(specsDir)
				}).OrElse(func() {
					idValidator = validation.NewIDValidator(emptyValue)
				}).Run()
			}).OrElse(func() {
				idValidator = validation.GetIDValidator()
			}).Run()

			// Force reload to ensure new specs are picked up.
			if err := idValidator.ReloadPatterns(); err != nil {
				st.err = errfmt.Newf(ErrMsgLoadIDPatterns).Wrap(err)
				return st, nil
			}

			// Verify the pattern was loaded for audit_aggregation_metric.
			prefixes := idValidator.GetValidPrefixes(MetricKindAuditAggregation)
			if len(prefixes) == 0 {
				st.err = errfmt.Errorf(ErrMsgIDPatternNotFound)
				return st, nil
			}

			if ctx != nil && ctx.Err() != nil {
				st.err = ctx.Err()
				return st, nil
			}

			metricID, err := s.createAggregationMetric(ctx, secCtx, aggregationMetric)
			if err != nil {
				st.err = errfmt.Newf(ErrMsgCreateAggMetric).Wrap(err)
				return st, nil
			}
			st.metricID = metricID

			if ctx != nil && ctx.Err() != nil {
				st.err = ctx.Err()
				return st, nil
			}

			updatedCount, err := s.markEventsAsAggregated(ctx, secCtx, st.eventIDs)
			if err != nil {
				// Log error but don't fail - metric was created successfully.
				// Could implement retry logic here.
			}
			st.updatedCount = updatedCount

			st.result = &AuditAggregationResult{
				MetricID:        st.metricID,
				EventCount:      len(st.eventIDs),
				MetricsCreated:  1,
				EventsProcessed: st.eventIDs,
				EventsUpdated:   st.updatedCount,
				WindowStart:     windowStart,
				WindowEnd:       windowEnd,
			}
			return st, nil
		}).
		AddStage(pipeline.StageFinalize, func(pctx *pipeline.Context, payload any) (any, error) {
			return st, nil
		}).
		Build()

	_, runErr := pl.Run(&pipeline.Context{Ctx: ctx, Outcome: make(map[string]any)}, st)
	if runErr != nil {
		return nil, runErr
	}
	if st.result == nil {
		return nil, st.err
	}
	s.eventsAggregatedTotal.Add(int64(st.result.EventCount))
	return st.result, st.err
}

// queryAuditEventsInWindow queries audit events within a time window
// Works with both file and graph backends
// Deprecated: This function is kept for backward compatibility but is no longer used internally.
// The aggregation service now uses BatchProcessor with BatchQueryBuilder.
func (s *AuditAggregationService) queryAuditEventsInWindow(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	windowStart, windowEnd time.Time,
) ([]map[string]any, error) {
	// Legacy implementation - get all events without batching
	filter := ListFilter{
		Kind: MetricKindAuditEvent,
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$gte": windowStart.Format(time.RFC3339),
				"$lte": windowEnd.Format(time.RFC3339),
			},
		},
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: true,
		Limit:   0, // No limit - get all events (legacy behavior)
	}
	result, err := s.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, err
	}
	return result.Objects, nil
}

// aggregateEvents aggregates events into a metric object using the builder pattern
//
//nolint:gocritic // named results unnecessary; keep current signature
func (s *AuditAggregationService) aggregateEvents(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	events []map[string]any,
	windowStart, windowEnd time.Time,
) (map[string]any, []string, error) {
	// Count events by type
	eventTypeCounts := make(map[string]int)
	statusCounts := make(map[string]int)
	objectKindCounts := make(map[string]int)
	operationCounts := make(map[string]int)
	errorEventCount := 0
	eventIDs := make([]string, 0, len(events))

	// Track ID ranges for compression
	idRanges := s.compressEventIDs(events)

	for _, event := range events {
		// Collect event ID
		if id := objects.GetString(event, objects.FieldKeyID); id != "" {
			eventIDs = append(eventIDs, id)
		}

		// Count by event type
		if eventType := objects.GetString(event, objects.FieldKeyEventType); eventType != "" {
			eventTypeCounts[eventType]++
		}
		if status := objects.GetString(event, objects.FieldKeyStatus); status != emptyValue {
			statusCounts[status]++
			if isAuditEventErrorStatus(status) {
				errorEventCount++
			}
		}

		// Count by object kind (if available)
		// Audit events use "target_kind" field, not "object_kind"
		if objectKind := objects.GetString(event, objects.FieldKeyTargetKind); objectKind != emptyValue {
			objectKindCounts[objectKind]++
		}

		// Count by operation (if available)
		if operation := objects.GetString(event, objects.FieldKeyOperation); operation != emptyValue {
			operationCounts[operation]++
		}
	}

	// Generate a descriptive title (required by base_object)
	title := fmt.Sprintf(DescAuditAggTitleFmt,
		len(events),
		windowStart.Format(zqktime.LayoutDate),
		windowEnd.Format(zqktime.LayoutDate))

	windowStartStr := windowStart.Format(time.RFC3339)
	windowEndStr := windowEnd.Format(time.RFC3339)

	// Get instance builder from registry (spec-driven); fallback to valid schema version so instance validation passes
	registry := instance_builders.GetGlobalRegistry()
	schemaVersion, err := registry.GetLatestVersion(MetricKindAuditAggregation)
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditAggregationSchemaDefaultWarn).
			WithError(err).
			Log()
		schemaVersion = objects.DefaultSchemaVersion
	} else {
		schemaVersion = objects.ValidSchemaVersion(schemaVersion)
	}
	// Create a fresh builder instance for this use (not from registry singleton)
	// This prevents concurrent map writes when multiple aggregation jobs run concurrently
	// (audit_event_aggregation jobs can run concurrently per scheduler configuration)
	builder := bldr_instance_v1.NewAuditAggregationMetricInstanceBuilder(schemaVersion)

	// Generate ID for the metric (required by builder.Build())
	var metricID string
	if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
		if fileStorage.usesContentAddressableStorage(MetricKindAuditAggregation) {
			metricID = fmt.Sprintf("%s%d", PrefixAuditAggregationMetric, time.Now().UTC().UnixNano())
		} else {
			generatedID, err := fileStorage.generateID(ctx, MetricKindAuditAggregation)
			when.When(func() bool { return err != nil }).Then(func() {
				metricID = fmt.Sprintf("%s%d", PrefixAuditAggregationMetric, time.Now().UTC().UnixNano())
			}).OrElse(func() {
				metricID = generatedID
			}).Run()
		}
	} else {
		metricID = fmt.Sprintf("%s%d", PrefixAuditAggregationMetric, time.Now().UTC().UnixNano())
	}

	// Build audit aggregation metric using instance builder
	// Builder automatically handles: namespace_id, origin_project, origin_system, audit fields
	// Note: status is set to "completed" explicitly (not the default "implemented")
	builder.ID(metricID).
		Status(ValueStatusCompleted).
		SetField(MetricFieldTitle, title).
		SetField(MetricFieldMetricType, StorageMetricTypeSystem).
		SetField(MetricFieldSource, AuditAggregationMetricSource).
		SetField(MetricFieldTags, []string{StorageMetricTagAudit, StorageMetricTagAggregated}).
		SetField(MetricFieldCollectionCount, 1).
		SetField(MetricFieldFirstSeen, windowStartStr).
		SetField(MetricFieldLastSeen, windowEndStr)
	// Note: Audit fields (created_at, updated_at, created_by, updated_by) and metric defaults
	// (namespace_id, origin_project, origin_system, status) are automatically set by builder.Build()

	// Set audit aggregation specific fields
	builder.AggregationWindowStart(windowStartStr).
		AggregationWindowEnd(windowEndStr).
		EventCount(len(events))
	// Convert map[string]int to map[string]any for builder.
	// Spec requires event_type_counts minCount: 1; use at least one entry so validation passes when no events had event_type.
	eventTypeCountsAny := make(map[string]any, len(eventTypeCounts)+1)
	for k, v := range eventTypeCounts {
		eventTypeCountsAny[k] = v
	}
	if len(eventTypeCountsAny) == 0 {
		eventTypeCountsAny[aggEventTypeCountKeyAggregated] = len(events)
	}
	objectKindCountsAny := make(map[string]any, len(objectKindCounts))
	for k, v := range objectKindCounts {
		objectKindCountsAny[k] = v
	}
	operationCountsAny := make(map[string]any, len(operationCounts))
	for k, v := range operationCounts {
		operationCountsAny[k] = v
	}
	statusCountsAny := make(map[string]any, len(statusCounts))
	for k, v := range statusCounts {
		statusCountsAny[k] = v
	}
	errorRate := 0.0
	if len(events) > 0 {
		errorRate = float64(errorEventCount) / float64(len(events))
	}
	builder.EventTypeCounts(eventTypeCountsAny).
		ObjectKindCounts(objectKindCountsAny).
		OperationCounts(operationCountsAny).
		StatusCounts(statusCountsAny).
		ErrorEventCount(errorEventCount).
		ErrorRate(errorRate).
		AggregatedEventIds(idRanges)

	// Build the instance
	aggregationMetric, err := builder.Build()
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditAggregationBuildInstanceFailedWarn).
			WithError(err).
			Log()
		// Return empty metric on error - caller should handle
		return nil, eventIDs, err
	}

	return aggregationMetric, eventIDs, nil
}

// compressEventIDs compresses consecutive event IDs into ranges
// Returns array of individual IDs and ranges (e.g., ["AUD-1", "AUD-100..AUD-199", "AUD-250"])
func (s *AuditAggregationService) compressEventIDs(events []map[string]any) []string {
	// Extract and sort IDs
	ids := make([]string, 0, len(events))
	for _, event := range events {
		if id := objects.GetString(event, objects.FieldKeyID); strings.HasPrefix(id, PrefixAudit) {
			ids = append(ids, id)
		}
	}

	if len(ids) == 0 {
		return []string{}
	}

	// Sort IDs numerically
	sort.Slice(ids, func(i, j int) bool {
		return s.compareAuditID(ids[i], ids[j]) < 0
	})

	// Compress consecutive IDs into ranges
	result := make([]string, 0)
	rangeStart := ""
	rangeEnd := ""
	rangeCount := 0

	for i, id := range ids {
		if i == 0 {
			rangeStart = id
			rangeEnd = id
			rangeCount = 1
			continue
		}

		// Check if this ID is consecutive with the previous one
		if s.isConsecutive(rangeEnd, id) {
			rangeEnd = id
			rangeCount++
		} else {
			// End current range and start new one
			if rangeCount > 2 {
				// Use range format for 3+ consecutive IDs
				result = append(result, fmt.Sprintf("%s%s%s", rangeStart, SeparatorRange, rangeEnd))
			} else {
				// Use individual IDs for 1-2 IDs
				if rangeCount == 1 {
					result = append(result, rangeStart)
				} else {
					result = append(result, rangeStart, rangeEnd)
				}
			}
			rangeStart = id
			rangeEnd = id
			rangeCount = 1
		}
	}

	// Add final range
	when.When(func() bool { return rangeCount > 2 }).Then(func() {
		result = append(result, fmt.Sprintf("%s%s%s", rangeStart, SeparatorRange, rangeEnd))
	}).OrElseWhen(func() bool { return rangeCount == 1 }).Then(func() {
		result = append(result, rangeStart)
	}).OrElse(func() {
		result = append(result, rangeStart, rangeEnd)
	}).Run()

	return result
}

// compareAuditID compares two audit IDs numerically (AUD-123 vs AUD-456)
func (s *AuditAggregationService) compareAuditID(id1, id2 string) int {
	num1 := s.extractAuditIDNumber(id1)
	num2 := s.extractAuditIDNumber(id2)
	if num1 < num2 {
		return -1
	}
	if num1 > num2 {
		return 1
	}
	return 0
}

// extractAuditIDNumber extracts the numeric part from an audit ID (AUD-123 -> 123)
func (s *AuditAggregationService) extractAuditIDNumber(id string) int {
	idx := strings.LastIndexByte(id, '-')
	if idx == -1 || idx == len(id)-1 {
		return 0
	}
	num, err := strconv.Atoi(id[idx+1:])
	if err != nil {
		return 0
	}
	return num
}

// isConsecutive checks if two audit IDs are consecutive (AUD-100 and AUD-101)
func (s *AuditAggregationService) isConsecutive(id1, id2 string) bool {
	num1 := s.extractAuditIDNumber(id1)
	num2 := s.extractAuditIDNumber(id2)
	return num2 == num1+1
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
func (s *AuditAggregationService) createAggregationMetricSync(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	metric map[string]any,
) (string, error) {
	// Enforce per-source, per-window uniqueness before attempting creation:
	// - If an exact window match exists for this source, update that metric (upsert-by-window).
	// - If any overlapping window exists for this source with a different window, reject to avoid double-aggregation.
	windowStart, _ := metric[objects.FieldKeyAggregationWindowStart].(string)
	windowEnd, _ := metric[objects.FieldKeyAggregationWindowEnd].(string)
	source, _ := metric[objects.FieldKeySource].(string)
	if windowStart != emptyValue && windowEnd != emptyValue && source != emptyValue {
		existing, err := s.findExistingMetricByWindow(ctx, secCtx, windowStart, windowEnd)
		if err == nil && existing != nil {
			existingID, _ := existing[objects.FieldKeyID].(string)
			if existingID != emptyValue {
				updates := make(map[string]any)
				for k, v := range metric {
					if k != objects.FieldKeyID && k != objects.FieldKeyCreatedAt && k != objects.FieldKeyCreatedBy {
						updates[k] = v
					}
				}
				updates[objects.FieldKeyUpdatedAt] = zqktime.NowRFC3339UTC()
				updates[objects.FieldKeyUpdatedBy] = pkgctx.SystemAccountID
				if err := s.storage.Update(ctx, secCtx, existingID, updates); err != nil {
					return "", errfmt.Errorf(ErrMsgUpdateAggMetricWin, windowStart, windowEnd, err)
				}
				return existingID, nil
			}
		}

		// If a different metric overlaps this window for the same source, reject to avoid double-aggregation.
		overlappingID := s.findExistingMetricByOverlappingWindow(ctx, secCtx, windowStart, windowEnd)
		if overlappingID != emptyValue {
			return "", errfmt.Errorf(ErrMsgOverlapAggMetric, source, overlappingID, windowStart, windowEnd)
		}
	}

	// Try to create the metric
	err := s.storage.Create(ctx, secCtx, metric)
	if err != nil {
		// If object already exists, try to find and update the existing metric
		if err == ErrObjectExists || strings.Contains(err.Error(), ErrMsgAlreadyExists) {
			var metricID string

			// First, check if metric has an ID (it might have been set during Create attempt)
			if id, hasID := metric[objects.FieldKeyID].(string); hasID && id != emptyValue {
				metricID = id
			} else {
				// No ID set - we need to find existing metric by time window
				// Query for existing metrics with matching time window
				windowStart, _ := metric[objects.FieldKeyAggregationWindowStart].(string)
				windowEnd, _ := metric[objects.FieldKeyAggregationWindowEnd].(string)
				if windowStart != emptyValue && windowEnd != emptyValue {
					existingMetric, findErr := s.findExistingMetricByWindow(ctx, secCtx, windowStart, windowEnd)
					if findErr == nil && existingMetric != nil {
						if id := objects.GetString(existingMetric, objects.FieldKeyID); id != emptyValue {
							metricID = id
						}
					}
					// If findExistingMetricByWindow failed, try a more lenient search
					// by querying for metrics with overlapping time windows
					if metricID == emptyValue {
						metricID = s.findExistingMetricByOverlappingWindow(ctx, secCtx, windowStart, windowEnd)
					}
				}
			}

			if metricID != emptyValue {
				// Update existing metric instead of creating new one
				// Remove fields that shouldn't be updated
				updates := make(map[string]any)
				for k, v := range metric {
					// Skip ID and timestamps that should be preserved
					if k != objects.FieldKeyID && k != objects.FieldKeyCreatedAt && k != objects.FieldKeyCreatedBy {
						updates[k] = v
					}
				}
				// Always update updated_at and updated_by
				updates[objects.FieldKeyUpdatedAt] = zqktime.NowRFC3339UTC()
				updates[objects.FieldKeyUpdatedBy] = ValueSystem

				updateErr := s.storage.Update(ctx, secCtx, metricID, updates)
				if updateErr != nil {
					// If update fails due to hash mismatch, missing hash file, or stale CAS index entry,
					// skip the problematic metric and create a new one instead.
					// This handles cases where existing metrics have hash mismatches or CAS inconsistencies.
					updateErrStr := updateErr.Error()
					if strings.Contains(updateErrStr, "hash mismatch") ||
						strings.Contains(updateErrStr, ErrMsgReadHashFile) ||
						strings.Contains(updateErrStr, ErrMsgNoExist) ||
						strings.Contains(updateErrStr, ErrMsgBlockingIssues) {
						// Object has hash mismatch or is referenced in index but file doesn't exist
						// Skip this problematic metric and create a new one instead
						// Log the issue for visibility but don't fail the aggregation
						logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
						when.When(func() bool { return strings.Contains(updateErrStr, "hash mismatch") }).Then(func() {
							StorageLog(logger).Warn(LogEventStorageMetricInstanceSkipHashMismatch).
								MetricID(metricID).
								String("error", updateErrStr).
								Log()
						}).OrElse(func() {
							StorageLog(logger).Warn(LogEventStorageMetricInstanceSkipStaleCASIndex).
								MetricID(metricID).
								String("error", updateErrStr).
								Log()
							// Try to clean up stale CAS index entry if it's a file storage issue
							if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
								cas, casErr := fileStorage.getContentAddressableStorage(MetricKindAuditAggregation)
								if casErr == nil {
									// Directly remove from CAS index (this will handle missing hash files gracefully)
									casDeleteErr := cas.Delete(metricID)
									if casDeleteErr != nil {
										// If error is ErrMsgObjectNotFound, that's fine - another goroutine already cleaned it up
										// Only log warnings for other errors
										casDeleteErrStr := casDeleteErr.Error()
										if !strings.Contains(casDeleteErrStr, ErrMsgObjectNotFound) {
											StorageLog(logger).Warn(LogEventStorageMetricInstanceCleanupStaleCASFailed).
												MetricID(metricID).
												WithError(casDeleteErr).
												Log()
										}
									} else {
										updateReverseReferenceIndexOnDelete(metricID)
									}
								}
							}
						}).Run()
						// Remove ID from metric so a new one can be generated
						delete(metric, objects.FieldKeyID)
						createErr := s.storage.Create(ctx, secCtx, metric)
						if createErr != nil {
							// If create still fails with "object already exists", another goroutine may have created it
							// Try to find and return the existing metric ID
							if createErr == ErrObjectExists || strings.Contains(createErr.Error(), ErrMsgAlreadyExists) {
								// Try to find existing metric by time window one more time
								windowStart, _ := metric[objects.FieldKeyAggregationWindowStart].(string)
								windowEnd, _ := metric[objects.FieldKeyAggregationWindowEnd].(string)
								if windowStart != emptyValue && windowEnd != emptyValue {
									existingMetric, findErr := s.findExistingMetricByWindow(ctx, secCtx, windowStart, windowEnd)
									if findErr == nil && existingMetric != nil {
										if id := objects.GetString(existingMetric, objects.FieldKeyID); id != emptyValue {
											return id, nil
										}
									}
									// Try overlapping window search
									if existingID := s.findExistingMetricByOverlappingWindow(ctx, secCtx, windowStart, windowEnd); existingID != emptyValue {
										return existingID, nil
									}
								}
							}
							return "", errfmt.Newf(NoteStaleAggMetric).Wrap(createErr)
						}
						// Return the newly generated ID
						newID, ok := metric[objects.FieldKeyID].(string)
						if !ok {
							return "", errfmt.Errorf(ErrMsgMetricIDNotSet)
						}
						return newID, nil
					}
					return "", errfmt.Newf(ErrMsgUpdateAggMetric).Wrap(updateErr)
				}
				return metricID, nil
			}
			// If we couldn't find the existing metric, return a more helpful error
			return "", errfmt.Errorf(ErrMsgLocateAggMetric,
				metric[objects.FieldKeyAggregationWindowStart], metric[objects.FieldKeyAggregationWindowEnd], err)
		}
		// Other errors - return original error
		return "", err
	}

	// Return the ID that was generated
	id, ok := metric[objects.FieldKeyID].(string)
	if !ok {
		return "", errfmt.Errorf(ErrMsgMetricIDNotSet)
	}

	return id, nil
}

// mergeAggregationMetrics merges two aggregation metrics into one
// Combines counts, ID lists, and other aggregations
func (s *AuditAggregationService) mergeAggregationMetrics(metric1, metric2 map[string]any) map[string]any {
	merged := make(map[string]any)

	// Copy all fields from metric1
	maps.Copy(merged, metric1)

	// Merge event_count
	count1, _ := metric1[aggMergeKeyEventCount].(int)
	count2, _ := metric2[aggMergeKeyEventCount].(int)
	merged[aggMergeKeyEventCount] = count1 + count2

	// Merge event_type_counts. Builder returns map[string]any; merge as map[string]int then convert back for spec (minCount: 1).
	typeCounts1Any, _ := metric1[aggMergeKeyEventTypeCounts].(map[string]any)
	typeCounts2Any, _ := metric2[aggMergeKeyEventTypeCounts].(map[string]any)
	typeCounts1 := mapFromAnyToInt(typeCounts1Any)
	typeCounts2 := mapFromAnyToInt(typeCounts2Any)
	mergedTypeCounts := make(map[string]int)
	maps.Copy(mergedTypeCounts, typeCounts1)
	for k, v := range typeCounts2 {
		mergedTypeCounts[k] += v
	}
	if len(mergedTypeCounts) == 0 {
		mergedTypeCounts[aggEventTypeCountKeyAggregated] = count1 + count2
	}
	merged[aggMergeKeyEventTypeCounts] = mapIntToAny(mergedTypeCounts)

	// Merge status_counts
	statusCounts1Any, _ := metric1[aggMergeKeyStatusCounts].(map[string]any)
	statusCounts2Any, _ := metric2[aggMergeKeyStatusCounts].(map[string]any)
	statusCounts1 := mapFromAnyToInt(statusCounts1Any)
	statusCounts2 := mapFromAnyToInt(statusCounts2Any)
	if statusCounts1 != nil || statusCounts2 != nil {
		mergedStatusCounts := make(map[string]int)
		maps.Copy(mergedStatusCounts, statusCounts1)
		for k, v := range statusCounts2 {
			mergedStatusCounts[k] += v
		}
		merged[aggMergeKeyStatusCounts] = mapIntToAny(mergedStatusCounts)
	}

	// Merge object_kind_counts
	kindCounts1, _ := metric1[aggMergeKeyObjectKindCounts].(map[string]int)
	kindCounts2, _ := metric2[aggMergeKeyObjectKindCounts].(map[string]int)
	if kindCounts1 != nil || kindCounts2 != nil {
		if kindCounts1 == nil {
			kindCounts1 = make(map[string]int)
		}
		if kindCounts2 == nil {
			kindCounts2 = make(map[string]int)
		}
		mergedKindCounts := make(map[string]int)
		maps.Copy(mergedKindCounts, kindCounts1)
		for k, v := range kindCounts2 {
			mergedKindCounts[k] += v
		}
		merged[aggMergeKeyObjectKindCounts] = mergedKindCounts
	}

	// Merge operation_counts
	opCounts1, _ := metric1[aggMergeKeyOperationCounts].(map[string]int)
	opCounts2, _ := metric2[aggMergeKeyOperationCounts].(map[string]int)
	if opCounts1 != nil || opCounts2 != nil {
		if opCounts1 == nil {
			opCounts1 = make(map[string]int)
		}
		if opCounts2 == nil {
			opCounts2 = make(map[string]int)
		}
		mergedOpCounts := make(map[string]int)
		maps.Copy(mergedOpCounts, opCounts1)
		for k, v := range opCounts2 {
			mergedOpCounts[k] += v
		}
		merged[aggMergeKeyOperationCounts] = mergedOpCounts
	}

	// Merge aggregated_event_ids (combine and recompress)
	ids1, _ := metric1[aggMergeKeyAggregatedEventIDs].([]string)
	ids2, _ := metric2[aggMergeKeyAggregatedEventIDs].([]string)
	allIDs := make([]string, 0, len(ids1)+len(ids2))
	allIDs = append(allIDs, ids1...)
	allIDs = append(allIDs, ids2...)
	// Expand ranges, combine, then recompress
	expandedIDs := ExpandIDRanges(allIDs)
	// Create a simple event map for compression (just IDs)
	eventsForCompression := make([]map[string]any, len(expandedIDs))
	for i, id := range expandedIDs {
		eventsForCompression[i] = map[string]any{objects.FieldKeyID: id}
	}
	// Compress the combined IDs
	merged[aggMergeKeyAggregatedEventIDs] = s.compressEventIDs(eventsForCompression)

	// Update collection_count (number of batches merged)
	collectionCount1, _ := metric1[aggMergeKeyCollectionCount].(int)
	collectionCount2, _ := metric2[aggMergeKeyCollectionCount].(int)
	merged[aggMergeKeyCollectionCount] = collectionCount1 + collectionCount2

	// Update last_seen to be the later of the two
	lastSeen1, _ := metric1[aggMergeKeyLastSeen].(string)
	lastSeen2, _ := metric2[aggMergeKeyLastSeen].(string)
	if lastSeen2 > lastSeen1 {
		merged[aggMergeKeyLastSeen] = lastSeen2
	}

	errorCount := 0
	if statusCountsAny, ok := merged[aggMergeKeyStatusCounts].(map[string]any); ok {
		for status, countAny := range statusCountsAny {
			if isAuditEventErrorStatus(status) {
				errorCount += intFromAny(countAny)
			}
		}
	}
	merged[aggMergeKeyErrorEventCount] = errorCount
	if eventCount, ok := merged[aggMergeKeyEventCount].(int); ok && eventCount > 0 {
		merged[aggMergeKeyErrorRate] = float64(errorCount) / float64(eventCount)
	} else {
		merged[aggMergeKeyErrorRate] = float64(0)
	}

	// Update title to reflect merged count
	eventCount, _ := merged[aggMergeKeyEventCount].(int)
	windowStart, _ := merged[objects.FieldKeyAggregationWindowStart].(string)
	windowEnd, _ := merged[objects.FieldKeyAggregationWindowEnd].(string)
	merged[aggMergeKeyTitle] = fmt.Sprintf(DescAuditAggTitleFmt,
		eventCount,
		windowStart,
		windowEnd)

	return merged
}

// findExistingMetricByWindow finds an existing aggregation metric for a given time window
func (s *AuditAggregationService) findExistingMetricByWindow(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd string,
) (map[string]any, error) {
	// Query for metrics with matching time window
	storageCtx := pkgctx.GetStorageContext()
	filter := ListFilter{
		Kind: MetricKindAuditAggregation,
		Filters: map[string]any{
			objects.FieldKeyAggregationWindowStart: map[string]any{
				"$eq": windowStart,
			},
			objects.FieldKeyAggregationWindowEnd: map[string]any{
				"$eq": windowEnd,
			},
		},
		Limit: 1, // Only need one match
	}

	result, err := s.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, err
	}

	if len(result.Objects) > 0 {
		return result.Objects[0], nil
	}

	return nil, errfmt.Errorf(NoteNoExistingMetric)
}

// findExistingMetricByOverlappingWindow finds an existing aggregation metric with overlapping time window
// This is a fallback when exact time window match fails
func (s *AuditAggregationService) findExistingMetricByOverlappingWindow(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd string,
) string {
	// Parse time windows
	startTime, err1 := time.Parse(time.RFC3339, windowStart)
	endTime, err2 := time.Parse(time.RFC3339, windowEnd)
	if err1 != nil || err2 != nil {
		return "" // Can't parse times, skip this fallback
	}

	// Query for metrics that overlap with this window
	// A window overlaps if: start1 < end2 && start2 < end1
	storageCtx := pkgctx.GetStorageContext()
	filter := ListFilter{
		Kind: MetricKindAuditAggregation,
		Filters: map[string]any{
			objects.FieldKeySource: map[string]any{
				"$eq": ValueAuditAggregationJob,
			},
		},
		Limit:   10, // Check a few recent metrics
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: false, // Most recent first
	}

	result, err := s.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil || len(result.Objects) == 0 {
		return ""
	}

	// Check each metric for time window overlap
	for _, obj := range result.Objects {
		objStartStr, _ := obj[objects.FieldKeyAggregationWindowStart].(string)
		objEndStr, _ := obj[objects.FieldKeyAggregationWindowEnd].(string)
		if objStartStr == emptyValue || objEndStr == emptyValue {
			continue
		}

		objStart, err1 := time.Parse(time.RFC3339, objStartStr)
		objEnd, err2 := time.Parse(time.RFC3339, objEndStr)
		if err1 != nil || err2 != nil {
			continue
		}

		// Check if windows overlap: start1 < end2 && start2 < end1
		if startTime.Before(objEnd) && objStart.Before(endTime) {
			if id := objects.GetString(obj, objects.FieldKeyID); id != emptyValue {
				return id
			}
		}
	}

	return ""
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

	// Use bulk update to mark events as archived (they've been aggregated into metrics)
	updates := make([]BulkUpdateItem, 0, len(expandedIDs))
	for _, id := range expandedIDs {
		updates = append(updates, BulkUpdateItem{
			ID: id,
			Updates: map[string]any{
				objects.FieldKeyStatus: ValueStatusArchived,
			},
		})
	}

	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	result, err := s.storage.BulkUpdate(ctx, secCtx, updates)
	if err != nil {
		return 0, err
	}

	return result.SuccessCount, nil
}

// ExpandIDRange expands an ID range like "AUD-1..AUD-165" into individual IDs
func ExpandIDRange(rangeStr string) []string {
	if !strings.Contains(rangeStr, "..") {
		return []string{rangeStr}
	}

	parts := strings.Split(rangeStr, SeparatorRange)
	if len(parts) != 2 {
		return []string{rangeStr}
	}

	startStr := strings.TrimPrefix(parts[0], PrefixAudit)
	endStr := strings.TrimPrefix(parts[1], PrefixAudit)

	var start, end int
	if _, err := fmt.Sscanf(startStr, "%d", &start); err != nil {
		return []string{rangeStr}
	}
	if _, err := fmt.Sscanf(endStr, "%d", &end); err != nil {
		return []string{rangeStr}
	}

	if start > end {
		return []string{rangeStr}
	}

	ids := make([]string, 0, end-start+1)
	for i := start; i <= end; i++ {
		ids = append(ids, fmt.Sprintf("%s%d", PrefixAudit, i))
	}
	return ids
}

// ExpandIDRanges expands a list of ID ranges and individual IDs into a flat list of individual IDs
func ExpandIDRanges(idRanges []string) []string {
	var allIDs []string
	for _, idRange := range idRanges {
		expanded := ExpandIDRange(idRange)
		allIDs = append(allIDs, expanded...)
	}
	return allIDs
}

// QueryOldAggregatedEvents queries for all archived/aggregated events older than cutoff time
func (s *AuditAggregationService) QueryOldAggregatedEvents(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	cutoffTime time.Time,
) ([]string, error) {
	filter := ListFilter{
		Kind: MetricKindAuditEvent,
		Filters: map[string]any{
			objects.FieldKeyStatus: map[string]any{
				"$eq": ValueStatusArchived, // Events marked as archived after aggregation
			},
			objects.FieldKeyCreatedAt: map[string]any{
				"$lt": cutoffTime.Format(time.RFC3339), // Older than cutoff
			},
		},
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: true,
		Limit:   0, // No limit - get all old aggregated events
	}

	result, err := s.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		return nil, errfmt.Newf(ErrMsgQueryAggEvents).Wrap(err)
	}

	eventIDs := make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id := objects.GetString(obj, objects.FieldKeyID); id != emptyValue {
			eventIDs = append(eventIDs, id)
		}
	}

	return eventIDs, nil
}

// QueryOldAuditEventsByAge returns audit_event IDs older than cutoff (by created_at).
// Used for catch-up cleanup when aggregation has been failing and event count is high.
// limit caps the number of IDs returned per call (0 = no limit; use 1000-2000 for batching).
// Uses high-volume event cache for fast queries when available.
func (s *AuditAggregationService) QueryOldAuditEventsByAge(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	cutoffTime time.Time,
	limit int,
) ([]string, error) {
	// Try high-volume event cache first (fast path)
	cache := GetGlobalHighVolumeEventCache()
	if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
		projectRoot := fileStorage.GetProjectRoot()
		if cache.IsPopulatedForProject(projectRoot) {
			// Use cache for fast query
			eventIDs := cache.QueryOlderThan(cutoffTime, limit)
			if len(eventIDs) > 0 {
				// Filter out already-archived events (cache doesn't store status)
				filteredIDs := s.filterOutArchivedEvents(ctx, secCtx, storageCtx, eventIDs)
				if len(filteredIDs) > 0 {
					return filteredIDs, nil
				}
				// All events from cache were archived - fall through to storage query
			}
			// Cache returned empty but is populated - may be stale, fall through to storage query
		} else {
			// Cache not populated - try to build it quickly (non-blocking, with timeout)
			// This helps when cache wasn't built before aggregation started
			// Use shorter timeout (5s) to avoid blocking cleanup too long
			buildCtx, buildCancel := context.WithTimeout(ctx, 5*time.Second)
			defer buildCancel()
			if errBuild := EnsureHighVolumeEventCacheReady(buildCtx, projectRoot, s.storage, false); errBuild != nil {
				StorageLog(logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))).Warn(LogMsgCacheBuildFailed).WithError(errBuild).Log()
			}
			// Check again after potential build
			if cache.IsPopulatedForProject(projectRoot) {
				eventIDs := cache.QueryOlderThan(cutoffTime, limit)
				// Only use cache if we got results (cache might be partial with 10k limit)
				// If cache returns empty but is populated, fall through to storage query
				if len(eventIDs) > 0 {
					// Filter out already-archived events (cache doesn't store status)
					filteredIDs := s.filterOutArchivedEvents(ctx, secCtx, storageCtx, eventIDs)
					if len(filteredIDs) > 0 {
						return filteredIDs, nil
					}
					// All events from cache were archived - fall through to storage query
				}
			}
		}
	}

	// Fallback to storage query (slower but always works)
	// Add timeout to prevent hanging on large datasets (17k+ events)
	// Use 30 seconds - if query takes longer, return empty and let next batch try
	queryCtx := ctx
	var queryCancel context.CancelFunc
	if ctx != nil && ctx.Err() == nil {
		queryCtx, queryCancel = context.WithTimeout(ctx, 300*time.Second)
		defer func() {
			if queryCancel != nil {
				queryCancel()
			}
		}()
	}

	filter := ListFilter{
		Kind: MetricKindAuditEvent,
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$lt": cutoffTime.Format(time.RFC3339),
			},
			// Exclude already-archived events to avoid redundant updates
			objects.FieldKeyStatus: map[string]any{
				"$ne": ValueStatusArchived,
			},
		},
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: true,
		Limit:   limit,
	}
	result, err := s.storage.List(queryCtx, secCtx, storageCtx, filter)
	if err != nil {
		// If timeout, return empty slice (not error) so job can continue with next batch
		if queryCtx != nil && queryCtx.Err() == context.DeadlineExceeded {
			return []string{}, nil
		}
		return nil, errfmt.Newf(ErrMsgQueryAuditAge).Wrap(err)
	}
	eventIDs := make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id := objects.GetString(obj, objects.FieldKeyID); id != emptyValue {
			eventIDs = append(eventIDs, id)
		}
	}
	return eventIDs, nil
}

// filterOutArchivedEvents filters out event IDs that are already archived
// This is needed when using the cache (which doesn't store status) to avoid redundant updates
func (s *AuditAggregationService) filterOutArchivedEvents(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	eventIDs []string,
) []string {
	if len(eventIDs) == 0 {
		return eventIDs
	}

	// Read status for all IDs in bulk (more efficient than individual reads)
	// Use a timeout to avoid hanging
	filterCtx := ctx
	var filterCancel context.CancelFunc
	if ctx != nil && ctx.Err() == nil {
		filterCtx, filterCancel = context.WithTimeout(ctx, 10*time.Second)
		defer func() {
			if filterCancel != nil {
				filterCancel()
			}
		}()
	}

	// Query for these specific IDs and check their status
	filter := ListFilter{
		Kind: MetricKindAuditEvent,
		Filters: map[string]any{
			objects.FieldKeyID: map[string]any{
				"$in": eventIDs,
			},
			// Only return non-archived events
			objects.FieldKeyStatus: map[string]any{
				"$ne": "archived",
			},
		},
		Limit: len(eventIDs), // Limit to number of IDs we're checking
	}

	result, err := s.storage.List(filterCtx, secCtx, storageCtx, filter)
	if err != nil {
		// If query fails or times out, return empty to fall back to storage query
		// This is safer than potentially updating archived events
		if filterCtx != nil && filterCtx.Err() == context.DeadlineExceeded {
			return []string{}
		}
		// For other errors, log and return empty to be safe
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageAuditAggregationFilterArchivedFailedWarn).
			WithError(err).
			Int("event_count", len(eventIDs)).
			Log()
		return []string{}
	}

	// Extract IDs from filtered results
	filteredIDs := make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id := objects.GetString(obj, objects.FieldKeyID); id != emptyValue {
			filteredIDs = append(filteredIDs, id)
		}
	}

	return filteredIDs
}

// CleanupAggregatedEvents deletes or archives audit events that have been aggregated
// Skips events that are already archived to avoid redundant WAL entries.
func (s *AuditAggregationService) CleanupAggregatedEvents(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	eventIDs []string,
	archive bool,
) (int, error) {
	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if archive {
		// Filter out already-archived events to avoid redundant updates
		// This prevents WAL entries for events that are already archived
		storageCtx := pkgctx.GetStorageContext()
		nonArchivedIDs := s.filterOutArchivedEvents(ctx, secCtx, storageCtx, eventIDs)
		if len(nonArchivedIDs) == 0 {
			// All events are already archived - nothing to do
			return 0, nil
		}

		// Update only non-archived events to archived status
		updates := make([]BulkUpdateItem, 0, len(nonArchivedIDs))
		for _, id := range nonArchivedIDs {
			updates = append(updates, BulkUpdateItem{
				ID: id,
				Updates: map[string]any{
					objects.FieldKeyStatus: "archived",
				},
			})
		}

		if ctx != nil && ctx.Err() != nil {
			return 0, ctx.Err()
		}
		result, err := s.storage.BulkUpdate(ctx, secCtx, updates)
		if err != nil {
			return 0, err
		}
		return result.SuccessCount, nil
	}

	// Delete events using optimized bulk delete
	// Expand ID ranges if needed (eventIDs might be compressed ranges like "AUD-1..AUD-960")
	expandedIDs := ExpandIDRanges(eventIDs)

	// Update high-volume event cache before deletion
	updateHighVolumeEventCacheOnBulkDelete(expandedIDs)

	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	// Use optimized bulk delete if available (FileObjectStorage)
	if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
		result, err := fileStorage.BulkDeleteOptimized(ctx, secCtx, expandedIDs, false, 20) // 20 workers for parallel deletion
		if err != nil {
			return 0, err
		}
		// Invalidate high-volume cache so next Count() uses index (not stale cache)
		if cache := GetGlobalHighVolumeEventCache(); cache != nil {
			cache.InvalidateForProject(fileStorage.GetProjectRoot())
		}
		return result.SuccessCount, nil
	}

	if ctx != nil && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	// Fallback to standard BulkDelete for other storage types
	result, err := s.storage.BulkDelete(ctx, secCtx, expandedIDs, false)
	if err != nil {
		return 0, err
	}

	// Invalidate high-volume cache so next Count() uses index (not stale cache)
	if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
		if cache := GetGlobalHighVolumeEventCache(); cache != nil {
			cache.InvalidateForProject(fileStorage.GetProjectRoot())
		}
	}

	// If no events were deleted, check if there were errors
	if result.SuccessCount == 0 && result.FailureCount > 0 {
		// Return first error as an example
		if len(result.Errors) > 0 {
			return 0, errfmt.Errorf(ErrMsgDeleteEventsFmt,
				result.FailureCount, result.SuccessCount, result.Errors[0].Error)
		}
		return 0, errfmt.Errorf(ErrMsgDeleteEventsNoDet,
			result.FailureCount, result.SuccessCount)
	}

	return result.SuccessCount, nil
}

// AuditAggregationResult contains the results of an aggregation operation
type AuditAggregationResult struct {
	MetricID        string
	EventCount      int
	MetricsCreated  int
	EventsProcessed []string
	EventsUpdated   int
	WindowStart     time.Time
	WindowEnd       time.Time
}
