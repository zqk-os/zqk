package scheduler

import (
	"context"
	"strings"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/objects"
	storagepkg "github.com/zqk-os/zqk/pkg/storage"
)

// ChangeJournalAggregationHandler aggregates change journal entries
type ChangeJournalAggregationHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

// preExecutionHealthCheck performs health checks before job execution
// This helps identify issues that could cause job failures
func (h *ChangeJournalAggregationHandler) preExecutionHealthCheck(ctx context.Context, job *ScheduledJob, metricKind string) error {
	return preExecutionHealthCheck(ctx, h.storage, h.logger, job, metricKind)
}

// NewChangeJournalAggregationHandler creates a new change journal aggregation handler
func NewChangeJournalAggregationHandler(storage storagepkg.ObjectStorageProvider) ChangeJournalAggregationHandlerInterface {
	return &ChangeJournalAggregationHandler{
		storage: storage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute aggregates change journal entries via the standardized pipeline (INGEST → NORMALIZE → COMMIT → FINALIZE).
func (h *ChangeJournalAggregationHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	ChangeJournalAggregationLog(h.logger).Info(LogEventChangeJournalAggregationStarted).
		JobID(job.ID).
		Log()

	result, err := RunChangeJournalAggregationViaPipeline(ctx, h, job)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, aggErrHashMismatch) || strings.Contains(errStr, aggErrReadHashFile) {
			ChangeJournalAggregationLog(h.logger).Warn(LogEventChangeJournalAggregationHashMismatchPartialContinue).
				WithFields(jobLogFieldsWithErr(job, err)...).
				Log()
			if result == nil {
				return errfmt.Newf("aggregation failed with hash mismatch, will retry").Wrap(err)
			}
		} else if strings.Contains(errStr, "object already exists") || strings.Contains(errStr, "stale CAS index") {
			ChangeJournalAggregationLog(h.logger).Warn(LogEventChangeJournalAggregationAggregationMetricStaleCASOK).
				WithFields(jobLogFieldsWithErr(job, err)...).
				Log()
			return nil
		}
		ChangeJournalAggregationLog(h.logger).Error(LogEventChangeJournalAggregationAggregationFailed, err).
			JobID(job.ID).
			Log()
		return errfmt.Newf("aggregation failed").Wrap(err)
	}

	if result == nil {
		ChangeJournalAggregationLog(h.logger).Info(LogEventChangeJournalAggregationAggregationCompletedNoEntries).
			JobID(job.ID).
			Log()
		return nil
	}

	ChangeJournalAggregationLog(h.logger).Info(LogEventChangeJournalAggregationAggregationCompleted).
		JobID(job.ID).
		Int("entries_processed", result.EntryCount).
		Int("metrics_created", result.MetricsCreated).
		String("metric_id", result.MetricID).
		Log()
	return nil
}

// AggregationMetricsCleanupHandler cleans up old audit_aggregation_metric objects
type AggregationMetricsCleanupHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

// NewAggregationMetricsCleanupHandler creates a new aggregation metrics cleanup handler
func NewAggregationMetricsCleanupHandler(storage storagepkg.ObjectStorageProvider) AggregationMetricsCleanupHandlerInterface {
	return &AggregationMetricsCleanupHandler{
		storage: storage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute cleans up old aggregation metrics via the pipeline (INGEST → NORMALIZE → FINALIZE).
func (h *AggregationMetricsCleanupHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunAggregationMetricsCleanupViaPipeline(ctx, h, job)
}

// executeAggregationMetricsCleanupCore runs the cleanup logic. Called from RunAggregationMetricsCleanupViaPipeline NORMALIZE stage.
func (h *AggregationMetricsCleanupHandler) executeAggregationMetricsCleanupCore(ctx context.Context, job *ScheduledJob) error {
	AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupStarted).
		JobID(job.ID).
		Log()

	retentionDays := retentionDaysFromJobEnv(job.EnvironmentVariables, DefaultAggregationMetricsCleanupRetentionDays)

	if retentionDays <= 0 {
		AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupRetentionSkipNonPositive).
			JobID(job.ID).
			Log()
		return nil
	}

	cutoffTime := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)

	AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupCleaningOldMetrics).
		String(aggLogKeyCutoff, cutoffTime.Format(time.RFC3339)).
		Int(aggFieldRetentionDays, retentionDays).
		Log()

	// Get contexts
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Query for old aggregation metrics
	filter := storagepkg.ListFilter{
		Kind: objects.KindAuditAggregationMetric,
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				aggFilterOpLessThan: cutoffTime.Format(time.RFC3339), // Older than cutoff
			},
		},
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: true,
		Limit:   0, // No limit - get all old metrics
	}

	result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		AggregationMetricsCleanupLog(h.logger).Error(LogEventAggregationMetricsCleanupQueryOldMetricsFailed, err).
			JobID(job.ID).
			Log()
		return errfmt.Newf("failed to query old aggregation metrics").Wrap(err)
	}
	if result == nil || result.Objects == nil || len(result.Objects) == 0 {
		AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupNoOldMetricsFound).
			JobID(job.ID).
			String(aggLogKeyCutoff, cutoffTime.Format(time.RFC3339)).
			Log()
		return nil
	}

	// Collect metric IDs
	metricIDs := make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
			metricIDs = append(metricIDs, id)
		}
	}

	AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupFoundOldMetrics).
		JobID(job.ID).
		MetricsFound(len(metricIDs)).
		Log()

	// Delete old metrics using bulk delete
	// Use optimized bulk delete if available (FileObjectStorage)
	if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
		deleteResult, err := fileStorage.BulkDeleteOptimized(ctx, secCtx, metricIDs, false, 20)
		if err != nil {
			AggregationMetricsCleanupLog(h.logger).Error(LogEventAggregationMetricsCleanupDeleteOldMetricsFailed, err).
				JobID(job.ID).
				MetricsFound(len(metricIDs)).
				Log()
			return errfmt.Newf("failed to delete old aggregation metrics").Wrap(err)
		}

		AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupCleanedUpOldMetrics).
			JobID(job.ID).
			MetricsFound(len(metricIDs)).
			DeletedCount(deleteResult.SuccessCount).
			FailedCount(deleteResult.FailureCount).
			Log()

		if deleteResult.FailureCount > 0 {
			AggregationMetricsCleanupLog(h.logger).Warn(LogEventAggregationMetricsCleanupSomeMetricsDeleteFailed).
				JobID(job.ID).
				FailedCount(deleteResult.FailureCount).
				Log()
		}
	} else {
		// Fallback to standard BulkDelete
		deleteResult, err := h.storage.BulkDelete(ctx, secCtx, metricIDs, false)
		if err != nil {
			AggregationMetricsCleanupLog(h.logger).Error(LogEventAggregationMetricsCleanupDeleteOldMetricsFailed, err).
				JobID(job.ID).
				MetricsFound(len(metricIDs)).
				Log()
			return errfmt.Newf("failed to delete old aggregation metrics").Wrap(err)
		}

		AggregationMetricsCleanupLog(h.logger).Info(LogEventAggregationMetricsCleanupCleanedUpOldMetrics).
			JobID(job.ID).
			MetricsFound(len(metricIDs)).
			DeletedCount(deleteResult.SuccessCount).
			FailedCount(deleteResult.FailureCount).
			Log()

		if deleteResult.FailureCount > 0 {
			AggregationMetricsCleanupLog(h.logger).Warn(LogEventAggregationMetricsCleanupSomeMetricsDeleteFailed).
				JobID(job.ID).
				FailedCount(deleteResult.FailureCount).
				Log()
		}
	}

	return nil
}

// GenericMetricsCleanupHandler cleans up old metric objects of any kind
// The metric kind is specified via job environment variable METRIC_KIND
type GenericMetricsCleanupHandler struct {
	storage storagepkg.ObjectStorageProvider
	logger  logging.Logger
}

// NewGenericMetricsCleanupHandler creates a new generic metrics cleanup handler
func NewGenericMetricsCleanupHandler(storage storagepkg.ObjectStorageProvider) GenericMetricsCleanupHandlerInterface {
	return &GenericMetricsCleanupHandler{
		storage: storage,
		logger:  logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem)),
	}
}

// Execute cleans up old metrics of the specified kind via the pipeline (INGEST → NORMALIZE → FINALIZE).
func (h *GenericMetricsCleanupHandler) Execute(ctx context.Context, job *ScheduledJob) error {
	return RunGenericMetricsCleanupViaPipeline(ctx, h, job)
}

// executeGenericMetricsCleanupCore runs the cleanup logic. Called from RunGenericMetricsCleanupViaPipeline NORMALIZE stage.
func (h *GenericMetricsCleanupHandler) executeGenericMetricsCleanupCore(ctx context.Context, job *ScheduledJob) error {
	// Get metric kind from job environment variables (required)
	metricKind := ""
	if job.EnvironmentVariables != nil {
		if kind, ok := job.EnvironmentVariables[EnvKeyMetricKind]; ok && kind != emptyValue {
			metricKind = kind
		}
	}

	if metricKind == emptyValue {
		err := errfmt.Errorf("%s environment variable is required for generic metrics cleanup", EnvKeyMetricKind)
		GenericMetricsCleanupLog(h.logger).Error(LogEventGenericMetricsCleanupMetricKindEnvNotSet, err).
			JobID(job.ID).
			Log()
		return err
	}

	GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupStarted).
		JobID(job.ID).
		MetricKind(metricKind).
		Log()

	retentionDays := retentionDaysFromJobEnv(job.EnvironmentVariables, DefaultGenericMetricsCleanupRetentionDays)

	if retentionDays <= 0 {
		GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupRetentionSkipNonPositive).
			JobID(job.ID).
			MetricKind(metricKind).
			Log()
		return nil
	}

	// Get contexts
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// CRITICAL: Check object count before cleanup to enforce 3k limit
	// If over limit, use aggressive cleanup (reduce retention by 50%)
	metricFilter := storagepkg.ListFilter{
		Kind: metricKind,
	}
	metricCount, countErr := h.storage.Count(ctx, secCtx, metricFilter)
	if countErr == nil && metricCount >= MaxAggregationObjectCountPerKind {
		// Object count at or above limit - trigger aggressive cleanup
		GenericMetricsCleanupLog(h.logger).Warn(LogEventGenericMetricsCleanupObjectCountLimitAggressive).
			JobID(job.ID).
			MetricKind(metricKind).
			Int(aggLogKeyCurrentCount, metricCount).
			Int(aggLogKeyLimit, MaxAggregationObjectCountPerKind).
			Log()

		// Reduce retention duration to 50% to free up space faster
		retentionDays /= 2
		if retentionDays < 1 {
			retentionDays = 1 // Minimum 1 day retention
		}
	}

	cutoffTime := time.Now().UTC().Add(-time.Duration(retentionDays) * 24 * time.Hour)

	GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupCleaningOldMetrics).
		JobID(job.ID).
		MetricKind(metricKind).
		String(aggLogKeyCutoff, cutoffTime.Format(time.RFC3339)).
		Int(aggFieldRetentionDays, retentionDays).
		Log()

	// Query for old metrics
	filter := storagepkg.ListFilter{
		Kind: metricKind,
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				aggFilterOpLessThan: cutoffTime.Format(time.RFC3339), // Older than cutoff
			},
		},
		SortBy:  objects.FieldKeyCreatedAt,
		SortAsc: true,
		Limit:   0, // No limit - get all old metrics
	}

	result, err := h.storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		GenericMetricsCleanupLog(h.logger).Error(LogEventGenericMetricsCleanupQueryOldMetricsFailed, err).
			JobID(job.ID).
			MetricKind(metricKind).
			Log()
		return errfmt.Newf("failed to query old metrics").Wrap(err)
	}
	if result == nil || result.Objects == nil || len(result.Objects) == 0 {
		GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupNoOldMetricsFound).
			JobID(job.ID).
			MetricKind(metricKind).
			String(aggLogKeyCutoff, cutoffTime.Format(time.RFC3339)).
			Log()
		return nil
	}

	// Collect metric IDs
	metricIDs := make([]string, 0, len(result.Objects))
	for _, obj := range result.Objects {
		if id, ok := obj[objects.FieldKeyID].(string); ok && id != emptyValue {
			metricIDs = append(metricIDs, id)
		}
	}

	GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupFoundOldMetrics).
		JobID(job.ID).
		MetricKind(metricKind).
		MetricsFound(len(metricIDs)).
		Log()

	// Delete old metrics using bulk delete
	// Use optimized bulk delete if available (FileObjectStorage)
	if fileStorage, ok := h.storage.(*storagepkg.FileObjectStorage); ok {
		deleteResult, err := fileStorage.BulkDeleteOptimized(ctx, secCtx, metricIDs, false, 20)
		if err != nil {
			GenericMetricsCleanupLog(h.logger).Error(LogEventGenericMetricsCleanupDeleteOldMetricsFailed, err).
				JobID(job.ID).
				MetricKind(metricKind).
				MetricsFound(len(metricIDs)).
				Log()
			return errfmt.Errorf(aggErrFailedDeleteOldMetricsFmt, err)
		}

		GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupCleanedUpOldMetrics).
			JobID(job.ID).
			MetricKind(metricKind).
			MetricsFound(len(metricIDs)).
			DeletedCount(deleteResult.SuccessCount).
			FailedCount(deleteResult.FailureCount).
			Log()

		if deleteResult.FailureCount > 0 {
			GenericMetricsCleanupLog(h.logger).Warn(LogEventGenericMetricsCleanupSomeMetricsDeleteFailed).
				JobID(job.ID).
				MetricKind(metricKind).
				FailedCount(deleteResult.FailureCount).
				Log()
		}
	} else {
		// Fallback to standard BulkDelete
		deleteResult, err := h.storage.BulkDelete(ctx, secCtx, metricIDs, false)
		if err != nil {
			GenericMetricsCleanupLog(h.logger).Error(LogEventGenericMetricsCleanupDeleteOldMetricsFailed, err).
				JobID(job.ID).
				MetricKind(metricKind).
				MetricsFound(len(metricIDs)).
				Log()
			return errfmt.Errorf(aggErrFailedDeleteOldMetricsFmt, err)
		}

		GenericMetricsCleanupLog(h.logger).Info(LogEventGenericMetricsCleanupCleanedUpOldMetrics).
			JobID(job.ID).
			MetricKind(metricKind).
			MetricsFound(len(metricIDs)).
			DeletedCount(deleteResult.SuccessCount).
			FailedCount(deleteResult.FailureCount).
			Log()

		if deleteResult.FailureCount > 0 {
			GenericMetricsCleanupLog(h.logger).Warn(LogEventGenericMetricsCleanupSomeMetricsDeleteFailed).
				JobID(job.ID).
				MetricKind(metricKind).
				FailedCount(deleteResult.FailureCount).
				Log()
		}
	}

	return nil
}

// preExecutionHealthCheck performs health checks before aggregation job execution
// Checks for common issues that could cause job failures:
// - Hash mismatches in existing metrics
// - Missing objects referenced in aggregations
// - CAS inconsistencies
func preExecutionHealthCheck(ctx context.Context, storage storagepkg.ObjectStorageProvider, logger logging.Logger, job *ScheduledJob, metricKind string) error {
	secCtx := pkgctx.NewSystemSecurityContext()
	storageCtx := pkgctx.NewStorageContext()

	// Check for existing metrics with potential issues
	// Limit to recent metrics (last 24 hours) to avoid performance impact
	filter := storagepkg.ListFilter{
		Kind: metricKind,
		Filters: map[string]any{
			objects.FieldKeyCreatedAt: map[string]any{
				"$gte": time.Now().UTC().Add(-24 * time.Hour).Format(time.RFC3339),
			},
		},
		Limit: 100, // Limit check to recent metrics
	}

	result, err := storage.List(ctx, secCtx, storageCtx, filter)
	if err != nil {
		// List failure is not critical - log and continue
		aggregationHealthLogRoot(logger, metricKind).Debug(healthLogEventListFailed(metricKind)).
			JobID(job.ID).
			WithError(err).
			MetricKind(metricKind).
			Log()
		return nil // Don't fail job on health check errors
	}
	if result == nil || result.Objects == nil {
		return nil
	}

	// Check a sample of metrics for hash mismatches
	// We don't check all metrics to avoid performance impact
	sampleSize := 10
	if len(result.Objects) < sampleSize {
		sampleSize = len(result.Objects)
	}

	issuesFound := 0
	for i := 0; i < sampleSize; i++ {
		obj := result.Objects[i]
		id, ok := obj[objects.FieldKeyID].(string)
		if !ok || id == emptyValue {
			continue
		}

		// Try to read the object - if it fails with hash mismatch, log it
		_, readErr := storage.Read(ctx, secCtx, id)
		if readErr != nil {
			errStr := readErr.Error()
			if strings.Contains(errStr, aggErrHashMismatch) || strings.Contains(errStr, aggErrReadHashFile) {
				issuesFound++
				aggregationHealthLogRoot(logger, metricKind).Warn(healthLogEventHashMismatchSample(metricKind)).
					JobID(job.ID).
					MetricID(id).
					MetricKind(metricKind).
					Log()
			}
		}
	}

	if issuesFound > 0 {
		aggregationHealthLogRoot(logger, metricKind).Warn(healthLogEventMetricsPotentialIssues(metricKind)).
			JobID(job.ID).
			MetricKind(metricKind).
			Int("issues_found", issuesFound).
			Int("sample_size", sampleSize).
			String("note", "Consider running system check to fix hash mismatches").
			Log()
		// Don't return error - health check is advisory
		// Job will attempt to handle these gracefully
	}

	return nil
}
