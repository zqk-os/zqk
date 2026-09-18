// Extracted from audit_aggregation.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"path/filepath"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/paths"
	"github.com/zqk-os/zqk/pkg/pipeline"
	"github.com/zqk-os/zqk/pkg/storage/audit"
	fileutil "github.com/zqk-os/zqk/pkg/utils/fileutil"
	"github.com/zqk-os/zqk/pkg/validation"
	"github.com/zqk-os/zqk/pkg/when"
)

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
						if audit.IndexEmpty(ids) {
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
			queryFilters := audit.WindowStatusFilters(windowStart, windowEnd)

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
				merged, ok := audit.MergeAnyMetrics(PrefixAudit, SeparatorRange, firstResult, secondResult)
				if !ok {
					return nil, errfmt.Errorf(ErrMsgInvalidMetricMerge)
				}
				return merged, nil
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
				st.result = audit.EmptyAggregationResult()
			}

			return st, nil
		}).
		AddStage(pipeline.StageCommit, func(pctx *pipeline.Context, payload any) (any, error) {
			if st.err != nil || st.result != nil {
				return st, nil
			}

			aggregationMetric, ok := audit.AsMetricMap(st.aggregationMetricAny)
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
				_, err := fileutil.Stat(specsDir)
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

			st.result = audit.CompletedAggregationResult(st.metricID, st.eventIDs, st.updatedCount, windowStart, windowEnd)
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
