package storage

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage/audit"
	"github.com/zqk-os/zqk/pkg/when"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

const pipelineKindAuditAggregationAggregateAuditEvents = "storage.audit_aggregation_aggregate_audit_events"

// auditEventAggregationAllowedStatuses are audit_event lifecycle statuses that are eligible for aggregation.
// Pending is excluded because it represents in-flight work; archived is excluded because it is a post-aggregation state.
var auditEventAggregationAllowedStatuses = audit.EligibleAggregationStatuses

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
func (s *AuditAggregationService) queryAuditEventsInWindow(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	windowStart, windowEnd time.Time,
) ([]map[string]any, error) {
	// Legacy implementation - get all events without batching
	return s.query().List(ctx, secCtx, storageCtx, audit.EventsOldestFirst(
		MetricKindAuditEvent,
		audit.CreatedAtInclusiveWindow(windowStart, windowEnd),
		0, // No limit - get all events (legacy behavior)
	))
}

// aggregateEvents aggregates events into a metric object using the builder pattern
//
//nolint:gocritic // named results unnecessary; keep current signature
func (s *AuditAggregationService) aggregateEvents(
	ctx context.Context,
	_ *pkgctx.SecurityContext,
	events []map[string]any,
	windowStart, windowEnd time.Time,
) (map[string]any, []string, error) {
	// Count events by type
	tally := audit.TallyEvents(events)
	eventIDs := tally.EventIDs

	// Track ID ranges for compression
	idRanges := s.compressEventIDs(events)

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
	builder.EventTypeCounts(tally.EventTypeCountsAny()).
		ObjectKindCounts(tally.ObjectKindCountsAny()).
		OperationCounts(tally.OperationCountsAny()).
		StatusCounts(tally.StatusCountsAny()).
		ErrorEventCount(tally.ErrorEventCount).
		ErrorRate(tally.ErrorRate()).
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
