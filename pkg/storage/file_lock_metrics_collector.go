package storage

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// File lock metric constants are now in metrics_constants.go

// FileLockMetricsCollector collects file lock metrics and creates metric objects
type FileLockMetricsCollector struct {
	storage               ObjectStorageProvider
	collectionsTotal      atomic.Int64
	metricsCollectedTotal atomic.Int64
}

// NewFileLockMetricsCollector creates a new file lock metrics collector
func NewFileLockMetricsCollector(storage ObjectStorageProvider) *FileLockMetricsCollector {
	return &FileLockMetricsCollector{
		storage: storage,
	}
}

// GetCollectorStats returns lifetime counters for collections attempted and metrics created.
func (c *FileLockMetricsCollector) GetCollectorStats() (collections, metrics int64) {
	return c.collectionsTotal.Load(), c.metricsCollectedTotal.Load()
}

// CollectMetrics creates a file_lock_metric object from current metrics snapshot
func (c *FileLockMetricsCollector) CollectMetrics(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
) (string, error) {
	c.collectionsTotal.Add(1)
	if !metricsrecording.Enabled() {
		return "", nil
	}
	snapshot, derived := GetFileLockSnapshotAndDerived()

	// Generate a descriptive title (required by base_object)
	title := fmt.Sprintf(ConstMiscFileLockMetricsDAcquisitionsFromSToS,
		snapshot.TotalAcquisitions,
		zqktime.FormatLayoutUTC(windowStart, zqktime.LayoutDateTimeSpace),
		zqktime.FormatLayoutUTC(windowEnd, zqktime.LayoutDateTimeSpace))

	now, windowStartStr, windowEndStr := zqktime.WindowStrings(windowStart, windowEnd)

	// Get instance builder from registry (factory pattern)
	// Get instance builder schema version from registry
	// NOTE: We create a fresh builder instance for each use to avoid concurrent map writes.
	// Builders from the registry are singleton instances with stateful fields maps that are not thread-safe.
	// While file lock metrics collection jobs are not concurrent, this prevents issues if concurrency is enabled in the future.
	schemaVersion, err := instance_builders.SchemaVersionForKind(MetricKindFileLockMetric)
	if err != nil {
		return "", errfmt.Newf(ConstMiscFailedToGetLatestSchemaVersionForFileLoc).Wrap(err)
	}

	builder := instance_builders.NewForKind(MetricKindFileLockMetric, schemaVersion)

	// Generate ID for the metric (required by builder.Build())
	// Uses thread-safe batch generator (via generateID) for non-CAS kinds, consistent with other ID generation
	// CAS-enabled kinds use timestamp-based IDs because CAS uses hash-based filenames
	var metricID string
	if fileStorage, ok := c.storage.(*FileObjectStorage); ok {
		// Check if this kind uses content-addressable storage
		// CAS-enabled kinds store files with hash-based names, so sequence-based ID generation can't scan the directory
		if fileStorage.usesContentAddressableStorage(MetricKindFileLockMetric) {
			// Use nanosecond-based timestamp ID for CAS-enabled kinds (ensures uniqueness even with concurrent execution)
			// UnixNano() provides nanosecond precision to avoid collisions when jobs run multiple times per second
			metricID = fmt.Sprintf("FLM-%d", now.UnixNano())
		} else {
			// Use thread-safe batch generator for non-CAS kinds (consistent with audit IDs and other sequential IDs)
			generatedID, err := fileStorage.generateID(ctx, MetricKindFileLockMetric)
			if err != nil {
				// Fallback to timestamp-based ID if generation fails (ensures uniqueness)
				metricID = fmt.Sprintf("FLM-%d", now.UnixNano())
			} else {
				metricID = generatedID
			}
		}
	} else {
		// For graph storage or other backends, use timestamp-based ID
		metricID = fmt.Sprintf("FLM-%d", now.UnixNano())
	}

	// Build file lock metric using instance builder
	// Builder automatically handles: created_at, updated_at, created_by, updated_by, namespace_id, origin_project, origin_system
	// Set status explicitly so lifecycle validation passes (use "completed" to align with metric lifecycles that allow it)
	builder.SetID(metricID).
		SetStatus("completed").
		SetField(MetricFieldTitle, title).
		SetField(MetricFieldMetricType, StorageMetricTypePerformance).
		SetField(MetricFieldSource, FileLockMetricSource).
		SetField(MetricFieldTags, []string{StorageMetricTagFileLock, StorageMetricTagConcurrency, StorageMetricTypePerformance}).
		SetField(MetricFieldCollectionCount, 1).
		SetField(MetricFieldFirstSeen, windowStartStr).
		SetField(MetricFieldLastSeen, windowEndStr)
	// Note: Audit fields (created_at, updated_at, created_by, updated_by) and metric defaults
	// (namespace_id, origin_project, origin_system, status) are automatically set by builder.Build()

	// Set file lock specific fields
	builder.SetField(objects.FieldKeyTotalAcquisitions, int(snapshot.TotalAcquisitions)).
		SetField(objects.FieldKeyTotalFailures, int(snapshot.TotalFailures)).
		SetField(objects.FieldKeyTotalTimeouts, int(snapshot.TotalTimeouts)).
		SetField(objects.FieldKeyTotalContention, int(snapshot.TotalContention)).
		SetField(objects.FieldKeyAvgAcquisitionTimeMs, float64(derived.AvgAcquisitionTime.Nanoseconds())/1e6).
		SetField(objects.FieldKeyAvgWaitTimeMs, float64(derived.AvgWaitTime.Nanoseconds())/1e6).
		SetField(objects.FieldKeyMaxAcquisitionTimeMs, float64(snapshot.MaxAcquisitionTime.Nanoseconds())/1e6).
		SetField(objects.FieldKeyMaxWaitTimeMs, float64(snapshot.MaxWaitTime.Nanoseconds())/1e6).
		SetField(objects.FieldKeyContentionRate, derived.ContentionRate*100).
		SetField(objects.FieldKeySuccessRate, derived.SuccessRate*100).
		SetField(objects.FieldKeyPeakContention, int(snapshot.PeakContention)).
		SetField(objects.FieldKeyMeasurementWindowStart, windowStartStr).
		SetField(objects.FieldKeyMeasurementWindowEnd, windowEndStr)

	// Build the instance
	instance, err := builder.Build()
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageMetricsFileLockBuildFailed).
			WithError(err).
			Log()
		return "", errfmt.Newf(ConstMiscFailedToBuildFileLockMetricInstance).Wrap(err)
	}

	// Create the metric object via storage (routes through CAS automatically)
	if err := c.storage.Create(ctx, secCtx, instance); err != nil {
		return "", errfmt.Newf(ConstMiscFailedToCreateFileLockMetric).Wrap(err)
	}

	c.metricsCollectedTotal.Add(1)

	// Return the generated ID
	return extractMetricInstanceID(instance)
}

// CollectAndReset collects metrics and resets the metrics counter
// This is useful for periodic collection (e.g., hourly, daily)
func (c *FileLockMetricsCollector) CollectAndReset(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
) (string, error) {
	return metricsrecording.CollectAndReset(
		func() (string, error) {
			return c.CollectMetrics(ctx, secCtx, windowStart, windowEnd)
		},
		ResetFileLockMetrics,
	)
}
