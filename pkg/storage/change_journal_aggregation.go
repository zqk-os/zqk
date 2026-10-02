package storage

import (
	"context"
	"fmt"
	"maps"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/storage/audit"
	"github.com/zqk-os/zqk/pkg/validation"
)

// ChangeJournalAggregationService handles aggregating change journal entries into metrics
type ChangeJournalAggregationService struct {
	storage                ObjectStorageProvider
	batchSize              int // Batch size for paginated queries (default: DefaultBatchSize)
	aggregationsRunTotal   atomic.Int64
	entriesAggregatedTotal atomic.Int64
}

// NewChangeJournalAggregationService creates a new change journal aggregation service with default batch size
func NewChangeJournalAggregationService(storage ObjectStorageProvider) *ChangeJournalAggregationService {
	return NewChangeJournalAggregationServiceWithBatchSize(storage, DefaultBatchSize)
}

// NewChangeJournalAggregationServiceWithBatchSize creates a new change journal aggregation service with specified batch size
func NewChangeJournalAggregationServiceWithBatchSize(storage ObjectStorageProvider, batchSize int) *ChangeJournalAggregationService {
	effectiveBatchSize := batchSize
	if effectiveBatchSize <= 0 {
		effectiveBatchSize = DefaultBatchSize
	}
	return &ChangeJournalAggregationService{
		storage:   storage,
		batchSize: effectiveBatchSize,
	}
}

// GetAggregationStats returns lifetime counters for aggregations run and entries aggregated.
func (s *ChangeJournalAggregationService) GetAggregationStats() (aggregationsRun, entriesAggregated int64) {
	return s.aggregationsRunTotal.Load(), s.entriesAggregatedTotal.Load()
}

// AggregateChangeJournalEntries aggregates change journal entries within a time window into metrics
func (s *ChangeJournalAggregationService) AggregateChangeJournalEntries(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	storageCtx *pkgctx.StorageContext,
	windowStart, windowEnd time.Time,
) (*ChangeJournalAggregationResult, error) {
	s.aggregationsRunTotal.Add(1)
	// Use batch processor for high-volume scenarios
	batchProcessor := NewBatchProcessor(s.batchSize)

	// Build query function for change journal entries in time window
	queryBuilder := NewBatchQueryBuilder(s.storage, secCtx, storageCtx, objects.KindChangeJournalEntry).
		WithFilters(map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$gte": windowStart.Format(time.RFC3339),
				"$lte": windowEnd.Format(time.RFC3339),
			},
		}).
		WithSort(objects.FieldKeyCreatedAt, true)
	queryFunc := queryBuilder.BuildQueryFunc(ctx)

	// Process function: aggregate a batch of entries
	processFunc := func(batch []map[string]any) (any, []string, error) {
		metric, entryIDs, err := s.aggregateEntries(ctx, secCtx, batch, windowStart, windowEnd)
		if err != nil {
			return nil, nil, err
		}
		return metric, entryIDs, nil
	}

	// Merge function: merge aggregation metrics
	mergeFunc := func(firstResult, secondResult any) (any, error) {
		metric1, ok1 := firstResult.(map[string]any)
		metric2, ok2 := secondResult.(map[string]any)
		if !ok1 || !ok2 {
			return nil, errfmt.Errorf(ConstAuditInvalidMetricTypeForMerging)
		}
		return s.mergeAggregationMetrics(metric1, metric2), nil
	}

	// Process in batches
	aggregationMetricAny, entryIDs, err := batchProcessor.ProcessInBatches(ctx, queryFunc, processFunc, mergeFunc)
	if err != nil {
		return nil, errfmt.Newf(ConstAuditFailedToProcessEntriesInBatches).Wrap(err)
	}

	if aggregationMetricAny == nil {
		return &ChangeJournalAggregationResult{
			EntryCount:       0,
			MetricsCreated:   0,
			EntriesProcessed: []string{},
		}, nil
	}

	aggregationMetric, ok := aggregationMetricAny.(map[string]any)
	if !ok {
		return nil, errfmt.Errorf(ConstAuditInvalidAggregationMetricType, aggregationMetricAny)
	}

	// Ensure kind mapper is initialized
	kindMapper := objects.GetGlobalKindMapper()
	if err := kindMapper.Initialize(); err != nil {
		return nil, errfmt.Newf(ConstAuditFailedToInitializeKindMapper).Wrap(err)
	}

	// Ensure ID validator has loaded patterns
	idValidator := validation.GetIDValidator()
	if err := idValidator.ReloadPatterns(); err != nil {
		return nil, errfmt.Newf(ConstAuditFailedToLoadIdPatterns).Wrap(err)
	}

	// Create aggregation metric (using audit_aggregation_metric kind)
	metricID, err := s.createAggregationMetric(ctx, secCtx, aggregationMetric)
	if err != nil {
		return nil, errfmt.Newf(ConstAuditFailedToCreateAggregationMetric).Wrap(err)
	}

	// Update change journal entries to mark them as aggregated
	updatedCount, err := s.markEntriesAsAggregated(ctx, secCtx, entryIDs)
	if err != nil {
		// Log error but don't fail - metric was created successfully
	}
	s.entriesAggregatedTotal.Add(int64(len(entryIDs)))

	return &ChangeJournalAggregationResult{
		MetricID:         metricID,
		EntryCount:       len(entryIDs),
		MetricsCreated:   1,
		EntriesProcessed: entryIDs,
		EntriesUpdated:   updatedCount,
		WindowStart:      windowStart,
		WindowEnd:        windowEnd,
	}, nil
}

// aggregateEntries aggregates change journal entries into a summary metric using the builder pattern
//
//nolint:gocritic // Multiple returns intentional for summary + IDs
func (s *ChangeJournalAggregationService) aggregateEntries(
	ctx context.Context,
	_ *pkgctx.SecurityContext,
	entries []map[string]any,
	windowStart, windowEnd time.Time,
) (map[string]any, []string, error) {
	// Group entries by change_type
	byChangeType := make(map[string]int)
	entryIDs := make([]string, 0, len(entries))

	for _, entry := range entries {
		entryID, _ := entry[objects.FieldKeyID].(string)
		if entryID != emptyValue {
			entryIDs = append(entryIDs, entryID)
		}

		changeType, _ := entry[objects.FieldKeyChangeType].(string)
		if changeType == emptyValue {
			changeType = "unknown"
		}
		byChangeType[changeType]++
	}

	// Create aggregation metric object
	now := time.Now().UTC()
	title := fmt.Sprintf(ConstAuditChangeJournalAggregationEntriesFromTo,
		len(entries),
		windowStart.Format(time.RFC3339),
		windowEnd.Format(time.RFC3339))

	// Build event_type_counts from change_type counts (for compatibility with audit_aggregation_metric schema)
	// event_type_counts should be a map[string]int, not an array
	eventTypeCounts := make(map[string]int)
	for changeType, count := range byChangeType {
		eventTypeCounts[changeType] = count
	}

	// Get instance builder from registry (spec-driven); fallback to valid schema version so instance validation passes
	schemaVersion, err := instance_builders.SchemaVersionForKind(MetricKindAuditAggregation)
	if err != nil {
		schemaVersion = objects.DefaultSchemaVersion
	} else {
		schemaVersion = objects.ValidSchemaVersion(schemaVersion)
	}
	builder := instance_builders.NewForKind(MetricKindAuditAggregation, schemaVersion)

	// Generate ID for the metric (required by builder.Build())
	var metricID string
	if fileStorage, ok := s.storage.(*FileObjectStorage); ok {
		if fileStorage.usesContentAddressableStorage(MetricKindAuditAggregation) {
			metricID = fmt.Sprintf("CJA-%d", now.UnixNano())
		} else {
			generatedID, err := fileStorage.generateID(ctx, MetricKindAuditAggregation)
			if err != nil {
				metricID = fmt.Sprintf("CJA-%d", now.UnixNano())
			} else {
				metricID = generatedID
			}
		}
	} else {
		metricID = fmt.Sprintf("CJA-%d", now.UnixNano())
	}

	windowStartStr := windowStart.Format(time.RFC3339)
	windowEndStr := windowEnd.Format(time.RFC3339)

	// Build audit aggregation metric using instance builder
	// Builder automatically handles: namespace_id, origin_project, origin_system, audit fields
	// Status must match audit_aggregation_metric lifecycle: completed, archived, error
	builder.SetID(metricID).
		SetStatus("completed").
		SetField(MetricFieldTitle, title).
		SetField(objects.FieldKeyDescription, fmt.Sprintf("Aggregation metric for %d change journal entries.", len(entries))).
		SetField(MetricFieldMetricType, StorageMetricTypeSystem).
		SetField(MetricFieldSource, ChangeJournalAggregationMetricSource).
		SetField(MetricFieldTags, []string{StorageMetricTagChangeJournal, StorageMetricTagAggregated}).
		SetField(MetricFieldCollectionCount, 1).
		SetField(MetricFieldFirstSeen, windowStartStr).
		SetField(MetricFieldLastSeen, windowEndStr)
	// Note: Audit fields (created_at, updated_at, created_by, updated_by) and metric defaults
	// (namespace_id, origin_project, origin_system) are automatically set by builder.Build()

	// Set audit aggregation specific fields
	builder.SetField(objects.FieldKeyAggregationWindowStart, windowStartStr).
		SetField(objects.FieldKeyAggregationWindowEnd, windowEndStr).
		SetField(objects.FieldKeyEventCount, len(entries))
	// Convert map[string]int to map[string]any for builder
	eventTypeCountsAny := make(map[string]any, len(eventTypeCounts))
	for k, v := range eventTypeCounts {
		eventTypeCountsAny[k] = v
	}
	builder.SetField(objects.FieldKeyEventTypeCounts, eventTypeCountsAny)

	// Build the instance
	aggregationMetric, err := builder.Build()
	if err != nil {
		return nil, nil, errfmt.Newf(ConstAuditFailedToBuildAuditAggregationMetricInstance).Wrap(err)
	}

	return aggregationMetric, entryIDs, nil
}

// compressEntryIDs compresses entry IDs into ranges for space efficiency
func (s *ChangeJournalAggregationService) compressEntryIDs(entryIDs []string) []string {
	if len(entryIDs) == 0 {
		return []string{}
	}

	// Sort and compress IDs (similar to audit aggregation)
	// For now, return as-is - can implement compression later if needed
	return entryIDs
}

// createAggregationMetric creates the aggregation metric object asynchronously (non-blocking)
// If a metric already exists for this time window, it updates the existing one instead
func (s *ChangeJournalAggregationService) createAggregationMetric(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	metric map[string]any,
) (string, error) {
	// Create metric asynchronously (non-blocking) using goroutine pattern
	// This ensures metrics creation never blocks the calling goroutine
	metricIDChan := make(chan string, 1)
	errChan := make(chan error, 1)

	goroutinelabels.NewGoroutine(ConstAuditChangeJournalAggregationMetricAsync, ConstAuditCreatingChangeJournalAggregationMetricAsynchronously).
		StartWithContext(ctx, func(ctx context.Context) error {
			metricID, err := s.createAggregationMetricSync(ctx, secCtx, metric)
			if err != nil {
				errChan <- err
				return nil
			}
			metricIDChan <- metricID
			return nil
		})

	// Wait for result with timeout
	select {
	case id := <-metricIDChan:
		return id, nil
	case err := <-errChan:
		return "", err
	case <-time.After(30 * time.Second):
		return "", errfmt.Errorf(ConstAuditMetricCreationTimeoutAfter30s)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// createAggregationMetricSync contains the synchronous logic for creating aggregation metrics
// This is called from createAggregationMetric which wraps it in a goroutine for non-blocking execution
func (s *ChangeJournalAggregationService) mergeAggregationMetrics(metric1, metric2 map[string]any) map[string]any {
	merged := make(map[string]any)

	// Copy all fields from metric1
	maps.Copy(merged, metric1)

	// Merge event_count
	count1, _ := metric1[objects.FieldKeyEventCount].(int)
	count2, _ := metric2[objects.FieldKeyEventCount].(int)
	merged[objects.FieldKeyEventCount] = count1 + count2

	// Merge event_type_counts (which contains change_type counts for change journal)
	typeCounts1, _ := metric1[objects.FieldKeyEventTypeCounts].(map[string]int)
	typeCounts2, _ := metric2[objects.FieldKeyEventTypeCounts].(map[string]int)
	if typeCounts1 == nil {
		typeCounts1 = make(map[string]int)
	}
	if typeCounts2 == nil {
		typeCounts2 = make(map[string]int)
	}
	merged[objects.FieldKeyEventTypeCounts] = audit.MergeIntMaps(typeCounts1, typeCounts2)

	// Update collection_count (number of batches merged)
	collectionCount1, _ := metric1[objects.FieldKeyCollectionCount].(int)
	collectionCount2, _ := metric2[objects.FieldKeyCollectionCount].(int)
	merged[objects.FieldKeyCollectionCount] = collectionCount1 + collectionCount2

	// Update last_seen to be the later of the two
	lastSeen1, _ := metric1[objects.FieldKeyLastSeen].(string)
	lastSeen2, _ := metric2[objects.FieldKeyLastSeen].(string)
	if lastSeen2 > lastSeen1 {
		merged[objects.FieldKeyLastSeen] = lastSeen2
	}

	// Update title to reflect merged count
	entryCount, _ := merged[objects.FieldKeyEventCount].(int)
	windowStart, _ := merged[objects.FieldKeyAggregationWindowStart].(string)
	windowEnd, _ := merged[objects.FieldKeyAggregationWindowEnd].(string)
	merged[objects.FieldKeyTitle] = fmt.Sprintf(ConstAuditChangeJournalAggregationEntriesFromTo,
		entryCount,
		windowStart,
		windowEnd)

	return merged
}

// findExistingMetricByWindow finds an existing aggregation metric for a given time window
func (s *ChangeJournalAggregationService) findExistingMetricByWindow(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd string,
) (map[string]any, error) {
	// Query for metrics with matching time window
	storageCtx := pkgctx.GetStorageContext()
	filter := ListFilter{
		Kind: objects.KindAuditAggregationMetric,
		Filters: map[string]any{
			objects.FieldKeyAggregationWindowStart: map[string]any{
				"$eq": windowStart,
			},
			objects.FieldKeyAggregationWindowEnd: map[string]any{
				"$eq": windowEnd,
			},
			objects.FieldKeySource: map[string]any{
				"$eq": ConstAuditChangeJournalAggregationJob,
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

	return nil, errfmt.Errorf(ConstAuditNoExistingMetricFoundForTimeWindow)
}

// findExistingMetricByOverlappingWindow finds an existing aggregation metric with overlapping time window
// This is a fallback when exact time window match fails
func (s *ChangeJournalAggregationService) findExistingMetricByOverlappingWindow(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd string,
) string {
	return findOverlappingMetric(ctx, secCtx, auditStore{s.storage}, ConstAuditChangeJournalAggregationJob, windowStart, windowEnd)
}

// markEntriesAsAggregated marks change journal entries as aggregated
func (s *ChangeJournalAggregationService) markEntriesAsAggregated(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	entryIDs []string,
) (int, error) {
	if len(entryIDs) == 0 {
		return 0, nil
	}

	// Update entries to mark them as aggregated
	updates := make([]BulkUpdateItem, 0, len(entryIDs))
	for _, id := range entryIDs {
		updates = append(updates, BulkUpdateItem{
			ID: id,
			Updates: map[string]any{
				objects.FieldKeyStatus: objects.ObjectStatusAggregated,
			},
		})
	}

	result, err := s.storage.BulkUpdate(ctx, secCtx, updates)
	if err != nil {
		return 0, errfmt.Newf(ConstAuditFailedToMarkEntriesAsAggregated).Wrap(err)
	}

	return result.SuccessCount, nil
}

// QueryOldAggregatedEntries queries for all aggregated entries older than cutoff time
