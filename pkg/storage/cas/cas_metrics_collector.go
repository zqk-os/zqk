package cas

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

// CASMetricsCollector collects CAS metrics and creates metric objects
type CASMetricsCollector struct {
	storage               MetricsStorageFacade
	collectionsTotal      atomic.Int64
	metricsCollectedTotal atomic.Int64
}

// NewCASMetricsCollector creates a new CAS metrics collector
func NewCASMetricsCollector(storage MetricsStorageFacade) *CASMetricsCollector {
	return &CASMetricsCollector{
		storage: storage,
	}
}

// GetCASCollectorStats returns lifetime counters for collections run and metrics collected.
func (c *CASMetricsCollector) GetCASCollectorStats() (collections, metrics int64) {
	return c.collectionsTotal.Load(), c.metricsCollectedTotal.Load()
}

// ObjectStorageMetricsCollector is a type alias for [CASMetricsCollector].
type ObjectStorageMetricsCollector = CASMetricsCollector

// NewObjectStorageMetricsCollector builds a collector for file-object storage metrics (same as [NewCASMetricsCollector]).
func NewObjectStorageMetricsCollector(storage MetricsStorageFacade) *ObjectStorageMetricsCollector {
	return NewCASMetricsCollector(storage)
}

// CollectMetrics creates a cas_metric object from current metrics snapshot
func (c *CASMetricsCollector) CollectMetrics(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
) (string, error) {
	c.collectionsTotal.Add(1)
	if !metricsrecording.Enabled() {
		return "", nil
	}
	metrics := GetObjectStorageMetrics()
	snapshot := metrics.GetSnapshot()

	// Generate a descriptive title (required by base_object)
	title := fmt.Sprintf(ConstStreamCasMetricsIntOperationsFromStrToStr,
		snapshot.Creates+snapshot.Reads+snapshot.Updates+snapshot.Deletes,
		zqktime.FormatLayoutUTC(windowStart, zqktime.LayoutDateTimeSpace),
		zqktime.FormatLayoutUTC(windowEnd, zqktime.LayoutDateTimeSpace))

	now := time.Now().UTC()
	windowStartStr := zqktime.FormatRFC3339UTC(windowStart)
	windowEndStr := zqktime.FormatRFC3339UTC(windowEnd)

	// TODO: Create cas_metric builder when schema is defined.
	// For now, use command_metric as a temporary solution (similar to audit_metrics).
	// Get instance builder from registry (factory pattern)
	schemaVersion, err := instance_builders.SchemaVersionForKind(objects.KindCommandMetric)
	if err != nil {
		return "", errfmt.Newf(ConstStreamFailedToGetLatestSchemaVersionForCommandMetric).Wrap(err)
	}

	// command_metric stands in until a cas_metric spec exists.
	builder := instance_builders.NewForKind(objects.KindCommandMetric, schemaVersion)

	// Generate ID for the metric (required by builder.Build())
	// Uses thread-safe batch generator (via generateID) for consistency with other ID generation
	// Note: If CAS is enabled for command_metric, timestamp-based IDs would be used instead
	var metricID string
	if fileStorage, ok := c.storage.(idGenerator); ok {
		// Check if CAS is enabled (CAS uses hash-based filenames, so sequence-based IDs don't work)
		if fileStorage.CASUsesContentAddressableStorage("command_metric") {
			// Use timestamp-based ID for CAS-enabled kinds (ensures uniqueness)
			metricID = fmt.Sprintf("CMD-%d", now.UnixNano())
		} else {
			// Use thread-safe batch generator for non-CAS kinds (consistent with audit IDs)
			generatedID, err := fileStorage.CASGenerateID(ctx, "command_metric")
			if err != nil {
				// Fallback to timestamp-based ID if generation fails
				metricID = fmt.Sprintf("CMD-%d", now.UnixNano())
			} else {
				metricID = generatedID
			}
		}
	} else {
		// For graph storage or other backends, use timestamp-based ID
		metricID = fmt.Sprintf("CMD-%d", now.UnixNano())
	}

	// Build CAS metric using command_metric builder (temporary until cas_metric schema exists)
	// Builder automatically handles: created_at, updated_at, created_by, updated_by, namespace_id, origin_project, origin_system, status
	builder.SetID(metricID).
		SetField(MetricFieldTitle, title).
		SetField(MetricFieldMetricType, StorageMetricTypePerformance).
		SetField(MetricFieldSource, CASSystemMetricSource).
		SetField(MetricFieldTags, []string{StorageMetricTagCAS, StorageMetricTagStorage, StorageMetricTypePerformance, StorageMetricTagContentAddressable}).
		SetField(MetricFieldCollectionCount, snapshot.Creates+snapshot.Reads+snapshot.Updates+snapshot.Deletes).
		SetField(MetricFieldFirstSeen, windowStartStr).
		SetField(MetricFieldLastSeen, windowEndStr)
	// Note: Audit fields (created_at, updated_at, created_by, updated_by) and metric defaults
	// (namespace_id, origin_project, origin_system, status) are automatically set by builder.Build()

	// Set command_metric fields (using CAS metrics as approximations)
	// NOTE: command_metric fields don't perfectly match CAS metrics, but we're using it
	// as a temporary solution. Ideally, this should use a dedicated cas_metric type.
	builder.SetField(objects.FieldKeyCommand, ConstStreamCasOperations).
		SetField(objects.FieldKeyNormalizedCmd, ConstStreamCasOperations)
	totalOps := snapshot.Creates + snapshot.Reads + snapshot.Updates + snapshot.Deletes
	totalFailures := snapshot.CreateFailures + snapshot.ReadFailures + snapshot.UpdateFailures + snapshot.DeleteFailures
	builder.SetField(objects.FieldKeyInvocationCount, int(totalOps)).
		SetField(objects.FieldKeySuccessCount, int(totalOps-totalFailures)).
		SetField(objects.FieldKeyFailureCount, int(totalFailures))

	// Calculate durations
	var avgTime time.Duration
	var maxTime time.Duration
	var minTime time.Duration

	if snapshot.AvgIndexTime > 0 {
		avgTime = snapshot.AvgIndexTime
	} else {
		// Fallback to average of all operations
		avgTime = (snapshot.AvgCreateTime + snapshot.AvgReadTime + snapshot.AvgUpdateTime + snapshot.AvgDeleteTime) / 4
	}

	if snapshot.MaxIndexTime > 0 {
		maxTime = snapshot.MaxIndexTime
	} else {
		maxTime = snapshot.MaxCreateTime
		if snapshot.MaxReadTime > maxTime {
			maxTime = snapshot.MaxReadTime
		}
		if snapshot.MaxUpdateTime > maxTime {
			maxTime = snapshot.MaxUpdateTime
		}
		if snapshot.MaxDeleteTime > maxTime {
			maxTime = snapshot.MaxDeleteTime
		}
	}

	// Find minimum time
	minTime = snapshot.MaxCreateTime
	if snapshot.MaxReadTime > 0 && (minTime == 0 || snapshot.MaxReadTime < minTime) {
		minTime = snapshot.MaxReadTime
	}
	if snapshot.MaxUpdateTime > 0 && (minTime == 0 || snapshot.MaxUpdateTime < minTime) {
		minTime = snapshot.MaxUpdateTime
	}
	if snapshot.MaxDeleteTime > 0 && (minTime == 0 || snapshot.MaxDeleteTime < minTime) {
		minTime = snapshot.MaxDeleteTime
	}

	builder.SetField(objects.FieldKeyAvgDurationSeconds, avgTime.Seconds()).
		SetField(objects.FieldKeySlowestDurationSeconds, maxTime.Seconds()).
		SetField(objects.FieldKeyFastestDurationSeconds, minTime.Seconds()).
		SetField(objects.FieldKeyBaselineDurationSeconds, avgTime.Seconds())

	// Calculate rates
	var errorRate float64
	if totalOps > 0 {
		errorRate = float64(totalFailures) / float64(totalOps) * 100
	}
	builder.SetField(objects.FieldKeyErrorRate, errorRate).
		SetField(objects.FieldKeyTimeoutRate, 0.0).
		SetField(objects.FieldKeyTimeoutCount, 0)

	// Content-addressed + listing-index fields as metadata (command_metric preserves extras).
	casMetadata := map[string]any{
		ConstStreamContentAddressedPuts:                   snapshot.Creates,
		ConstStreamContentAddressedReads:                  snapshot.Reads,
		ConstStreamContentAddressedUpdates:                snapshot.Updates,
		ConstStreamContentAddressedDeletes:                snapshot.Deletes,
		ConstStreamListingIndexSetMappings:                snapshot.SetMappings,
		ConstStreamListingIndexRemoveMappings:             snapshot.RemoveMappings,
		ConstStreamListingIndexLoads:                      snapshot.IndexLoads,
		ConstStreamListingIndexSaves:                      snapshot.IndexSaves,
		ConstStreamListingIndexReloads:                    snapshot.IndexReloads,
		ConstStreamListingIndexReloadsDuringSet:           snapshot.IndexReloadsDuringSetMapping,
		ConstStreamListingIndexMergeOperations:            snapshot.IndexMergeOperations,
		ConstStreamListingIndexEntriesAdded:               snapshot.IndexEntriesAdded,
		ConstStreamListingIndexEntriesRemoved:             snapshot.IndexEntriesRemoved,
		ConstStreamListingIndexSize:                       snapshot.IndexSize,
		ConstStreamListingIndexLockContention:             snapshot.IndexLockContention,
		ConstStreamListingIndexFileLockAcquisitions:       snapshot.IndexFileLockAcquisitions,
		ConstStreamListingIndexFileLockFailures:           snapshot.IndexFileLockFailures,
		ConstStreamContentAddressedAvgPutTimeMs:           snapshot.AvgCreateTime.Seconds() * 1000,
		ConstStreamContentAddressedAvgReadTimeMs:          snapshot.AvgReadTime.Seconds() * 1000,
		ConstStreamContentAddressedAvgUpdateTimeMs:        snapshot.AvgUpdateTime.Seconds() * 1000,
		ConstStreamContentAddressedAvgDeleteTimeMs:        snapshot.AvgDeleteTime.Seconds() * 1000,
		ConstStreamListingIndexAvgTimeMs:                  snapshot.AvgIndexTime.Seconds() * 1000,
		ConstStreamContentAddressedMaxPutTimeMs:           snapshot.MaxCreateTime.Seconds() * 1000,
		ConstStreamContentAddressedMaxReadTimeMs:          snapshot.MaxReadTime.Seconds() * 1000,
		ConstStreamContentAddressedMaxUpdateTimeMs:        snapshot.MaxUpdateTime.Seconds() * 1000,
		ConstStreamContentAddressedMaxDeleteTimeMs:        snapshot.MaxDeleteTime.Seconds() * 1000,
		ConstStreamListingIndexMaxTimeMs:                  snapshot.MaxIndexTime.Seconds() * 1000,
		ConstStreamListingIndexLockWaitTimeMs:             snapshot.IndexLockWaitTime.Seconds() * 1000,
		ConstStreamListingIndexFileLockWaitTimeMs:         snapshot.IndexFileLockWaitTime.Seconds() * 1000,
		ConstStreamContentAddressedMeasurementWindowStart: windowStartStr,
		ConstStreamContentAddressedMeasurementWindowEnd:   windowEndStr,
	}

	for k, v := range casMetadata {
		builder.SetField(k, v)
	}

	// Build the instance
	instance, err := builder.Build()
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageMetricsCASBuildFailed).
			WithError(err).
			Log()
		return "", errfmt.Newf(ConstStreamFailedToBuildCasMetricInstance).Wrap(err)
	}

	// Create the metric object via storage (routes through CAS automatically)
	if err := c.storage.Create(ctx, secCtx, instance); err != nil {
		return "", errfmt.Newf(ConstStreamFailedToCreateCasMetric).Wrap(err)
	}

	// Return the generated ID
	id, ok := instance[objects.FieldKeyID].(string)
	if !ok {
		return "", errfmt.Errorf(ConstStreamMetricIdNotSetAfterCreation)
	}

	c.metricsCollectedTotal.Add(1)
	return id, nil
}

// CollectAndReset collects metrics and resets the metrics counter
// This is useful for periodic collection (e.g., hourly, daily)
func (c *CASMetricsCollector) CollectAndReset(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
) (string, error) {
	if !metricsrecording.Enabled() {
		return "", nil
	}
	metricID, err := c.CollectMetrics(ctx, secCtx, windowStart, windowEnd)
	if err != nil {
		return "", err
	}

	// Reset metrics for next collection period
	ResetObjectStorageMetrics()

	return metricID, nil
}

type idGenerator interface {
	CASUsesContentAddressableStorage(kind string) bool
	CASGenerateID(ctx context.Context, kind string) (string, error)
}
