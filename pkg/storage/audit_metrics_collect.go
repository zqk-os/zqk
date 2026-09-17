// Extracted from audit_metrics.go (BLI-CEF-STORAGE-DECOMPOSE-001).
package storage

import (
	"context"
	"fmt"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metricsrecording"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/storage/audit"
	"github.com/lanceman/zqk/pkg/zqktime"
)

func (c *AuditMetricsCollector) CollectMetrics(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
) (string, error) {
	if !metricsrecording.Enabled() {
		return "", nil
	}
	snapshot := c.GetSnapshot()
	dur := snapshot.DurationSeconds()

	// Create metric object
	title := fmt.Sprintf(DescAuditMetricsTitleFmt,
		snapshot.EventsCreated,
		zqktime.FormatLayoutUTC(windowStart, zqktime.LayoutDateTimeSpace),
		zqktime.FormatLayoutUTC(windowEnd, zqktime.LayoutDateTimeSpace))

	now := time.Now().UTC()
	windowStartStr := zqktime.FormatRFC3339UTC(windowStart)
	windowEndStr := zqktime.FormatRFC3339UTC(windowEnd)

	// Get instance builder schema version from registry
	// NOTE: We create a fresh builder instance for each use to avoid concurrent map writes.
	// Builders from the registry are singleton instances with stateful fields maps that are not thread-safe.
	registry := instance_builders.GetGlobalRegistry()
	schemaVersion, err := registry.GetLatestVersion(objects.KindCommandMetric)
	if err != nil {
		return "", errfmt.Newf(ErrMsgGetLatestSchemaFmt).Wrap(err)
	}

	// Create a fresh builder instance for this use (not from registry singleton)
	builder := bldr_instance_v1.NewCommandMetricInstanceBuilder(schemaVersion)

	// Generate ID for the metric (required by builder.Build())
	// Uses thread-safe batch generator (via generateID) for consistency with other ID generation
	// Note: If CAS is enabled for command_metric, timestamp-based IDs would be used instead
	var generatedID string
	if fileStorage, ok := c.storage.(*FileObjectStorage); ok {
		if !fileStorage.usesContentAddressableStorage(objects.KindCommandMetric) {
			id, err := fileStorage.generateID(ctx, objects.KindCommandMetric)
			if err == nil {
				generatedID = id
			}
		}
	}
	metricID := audit.ChooseMetricID(generatedID, PrefixCommand, now)

	// Build command metric using instance builder
	// NOTE: This is conceptually wrong - command_metric is for command execution metrics,
	// not audit event metrics. The audit-specific fields are stored as metadata.
	builder.ID(metricID).
		SetField(MetricFieldTitle, title).
		SetField(MetricFieldMetricType, StorageMetricTypeSystem).
		SetField(MetricFieldSource, AuditSystemMetricSource).
		SetField(MetricFieldTags, []string{StorageMetricTagSystem, StorageMetricTagAudit, StorageMetricTagEvents}).
		SetField(MetricFieldCollectionCount, snapshot.EventsCreated).
		SetField(MetricFieldFirstSeen, windowStartStr).
		SetField(MetricFieldLastSeen, windowEndStr)
	// Builder automatically handles: status, namespace_id, origin_project, origin_system, audit fields
	// Note: Audit fields (created_at, updated_at, created_by, updated_by) and metric defaults
	// (namespace_id, origin_project, origin_system, status) are automatically set by builder.Build()

	// Set command metric fields (using audit metrics as approximations)
	// These fields don't perfectly match audit metrics, but we're using command_metric
	// as a temporary solution. Ideally, this should use a dedicated audit_metric type.
	builder.Command(ValueAuditEventCollection).
		NormalizedCmd(ValueAuditEventCollection).
		InvocationCount(int(snapshot.EventsCreated)).
		SuccessCount(int(snapshot.SuccessCount())).
		FailureCount(int(snapshot.EventsFailed)).
		AvgDurationSeconds(dur.Avg)

	builder.SlowestDurationSeconds(dur.Slowest)
	builder.FastestDurationSeconds(dur.Fastest)
	builder.BaselineDurationSeconds(dur.Baseline)

	builder.ErrorRate(snapshot.FailureRate()).
		TimeoutRate(0.0).
		TimeoutCount(0)

	// Build the instance
	metricObj, err := builder.Build()
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageMetricsCommandBuildFailed).
			WithError(err).
			Log()
		return "", errfmt.Newf(ErrMsgBuildCommandMetric).Wrap(err)
	}

	audit.AttachCommandMetricMetadata(metricObj, snapshot, windowEnd.Sub(windowStart).Seconds())

	if c.storage == nil {
		return "", errfmt.Errorf(ErrMsgStorageNotConfigured)
	}

	// Create metric asynchronously (non-blocking) using goroutine pattern
	// This ensures metrics creation never blocks the calling goroutine
	metricIDChan := make(chan string, 1)
	errChan := make(chan error, 1)

	goroutinelabels.NewGoroutine(OpNameAuditMetricsCollectorAsync, DescAuditMetricsAsync).
		StartWithContext(ctx, func(ctx context.Context) error {
			if err := c.metrics().Create(ctx, secCtx, metricObj); err != nil {
				errChan <- errfmt.Newf(ErrMsgCreateMetricObject).Wrap(err)
				return nil
			}

			// Return ID
			id, ok := metricObj[objects.FieldKeyID].(string)
			if !ok {
				errChan <- errfmt.Errorf(ErrMsgMetricIDNotSet)
				return nil
			}

			metricIDChan <- id
			return nil
		})

	return audit.AwaitCreate(ctx, DefaultAuditMetricTimeout, errfmt.Errorf(ErrMsgMetricTimeout, DefaultAuditMetricTimeout), metricIDChan, errChan)
}

// CollectAndReset collects metrics and resets the metrics counter
// This is useful for periodic collection (e.g., hourly, daily)
func (c *AuditMetricsCollector) CollectAndReset(
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
	c.eventsCreated.Store(0)
	c.eventsCreatedCAS.Store(0)
	c.eventsCreatedID.Store(0)
	c.eventsFailed.Store(0)
	c.eventsValidated.Store(0)
	c.eventsSkipped.Store(0)
	c.eventsDuplicated.Store(0)
	c.eventsMerged.Store(0)
	c.totalCreationTime.Store(0)
	c.maxCreationTime.Store(0)
	c.creationEvents.Store(0)
	c.updateEvents.Store(0)
	c.deleteEvents.Store(0)
	c.bulkEvents.Store(0)
	c.systemEvents.Store(0)
	c.otherEvents.Store(0)

	return metricID, nil
}
