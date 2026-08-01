package storage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
	"github.com/lanceman/zqk/pkg/metricsrecording"
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/observability"
	"github.com/lanceman/zqk/pkg/specbuilder/bldr_instance_v1"
	"github.com/lanceman/zqk/pkg/specbuilder/instance_builders"
	"github.com/lanceman/zqk/pkg/zqktime"
)

// AuditMetricsCollector collects metrics for audit event operations
type AuditMetricsCollector struct {
	storage ObjectStorageProvider

	// Event emission (ITEM-912 Phase 2: MetricPipeline integration)
	// EventEmitter is an optional function to emit events to metrics pipeline
	// If nil, events are not emitted
	eventEmitter func(event map[string]any) error
	emitEvents   atomic.Bool // Atomic flag: true = enabled, false = disabled (default)

	// New builder-pattern recorder (stored as any to avoid import cycles)
	recorder any // observability.Recorder

	// Counters (use atomic operations for thread-safety)
	eventsCreated    atomic.Int64
	eventsCreatedCAS atomic.Int64 // Events created via CAS (hash-based filenames)
	eventsCreatedID  atomic.Int64 // Events created with ID-based filenames (fallback)
	eventsFailed     atomic.Int64
	eventsValidated  atomic.Int64
	eventsSkipped    atomic.Int64 // Events skipped due to validation failures
	eventsDuplicated atomic.Int64 // Events that already existed (duplicate attempts)
	eventsMerged     atomic.Int64 // Duplicate events that were successfully merged

	// Timing metrics (in nanoseconds)
	totalCreationTime atomic.Int64
	maxCreationTime   atomic.Int64

	// Event type counters
	creationEvents atomic.Int64
	updateEvents   atomic.Int64
	deleteEvents   atomic.Int64
	bulkEvents     atomic.Int64
	systemEvents   atomic.Int64
	otherEvents    atomic.Int64
}

var (
	globalAuditMetrics     *AuditMetricsCollector
	globalAuditMetricsOnce sync.Once
)

// auditCmdMetricMetadata* are JSON/metadata map keys for command_metric audit snapshots (centralized to avoid repeated "key" index literals).
const (
	auditCmdMetricObjKeyMetadata         = MetricMetaMetadata
	auditCmdMetricMetaEventsCreatedCAS   = MetricMetaEventsCreatedCAS
	auditCmdMetricMetaEventsCreatedID    = MetricMetaEventsCreatedID
	auditCmdMetricMetaEventsValidated    = MetricMetaEventsValidated
	auditCmdMetricMetaEventsSkipped      = MetricMetaEventsSkipped
	auditCmdMetricMetaEventsDuplicated   = MetricMetaEventsDuplicated
	auditCmdMetricMetaEventsMerged       = MetricMetaEventsMerged
	auditCmdMetricMetaMaxCreationTimeMs  = MetricMetaMaxCreationTimeMs
	auditCmdMetricMetaCasUsageRate       = MetricMetaCasUsageRate
	auditCmdMetricMetaFailureRate        = MetricMetaFailureRate
	auditCmdMetricMetaValidationSkipRate = MetricMetaValidationSkipRate
	auditCmdMetricMetaCreationEvents     = MetricMetaCreationEvents
	auditCmdMetricMetaUpdateEvents       = MetricMetaUpdateEvents
	auditCmdMetricMetaDeleteEvents       = MetricMetaDeleteEvents
	auditCmdMetricMetaBulkEvents         = MetricMetaBulkEvents
	auditCmdMetricMetaSystemEvents       = MetricMetaSystemEvents
	auditCmdMetricMetaOtherEvents        = MetricMetaOtherEvents
	auditCmdMetricMetaMeasurementPeriod  = MetricMetaMeasurementPeriod
)

// GetGlobalAuditMetricsCollector returns the global audit metrics collector
func GetGlobalAuditMetricsCollector() *AuditMetricsCollector {
	globalAuditMetricsOnce.Do(func() {
		globalAuditMetrics = &AuditMetricsCollector{}
	})
	return globalAuditMetrics
}

// NewAuditMetricsCollector creates a new audit metrics collector
func NewAuditMetricsCollector(storage ObjectStorageProvider) *AuditMetricsCollector {
	return &AuditMetricsCollector{
		storage: storage,
	}
}

// SetEventEmitter sets the event emitter function for MetricPipeline integration (ITEM-912 Phase 2)
// Events are emitted asynchronously to avoid blocking audit event creation
func (c *AuditMetricsCollector) SetEventEmitter(emitter func(event map[string]any) error, enabled bool) {
	c.eventEmitter = emitter
	c.emitEvents.Store(enabled)
}

// emitEvent optionally emits an event to the metrics pipeline (ITEM-912 Phase 2)
func (c *AuditMetricsCollector) emitEvent(event map[string]any) {
	if !c.emitEvents.Load() || c.eventEmitter == nil {
		return
	}

	// Emit asynchronously to avoid blocking
	goroutinelabels.NewGoroutine(OpNameAuditMetricsEventEmitter, DescAuditMetricsEventEmitter).
		StartSimple(func() {
			if err := c.eventEmitter(event); err != nil {
				logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("%s: %v\n", ErrMsgEmitAuditMetric, err), nil).Log()
			}
		})
}

// SetEmitEvents enables or disables event emission to metrics pipeline
func (c *AuditMetricsCollector) SetEmitEvents(enabled bool) {
	c.emitEvents.Store(enabled)
}

// ShouldEmitEvents returns whether event emission is enabled
func (c *AuditMetricsCollector) ShouldEmitEvents() bool {
	return c.emitEvents.Load() && c.eventEmitter != nil
}

// RecordAuditEventCreation records metrics for an audit event creation
func (c *AuditMetricsCollector) RecordAuditEventCreation(
	ctx context.Context,
	eventType string,
	usedCAS bool,
	duration time.Duration,
	success bool,
) {
	if !metricsrecording.Enabled() {
		return
	}
	c.eventsCreated.Add(1)
	if usedCAS {
		c.eventsCreatedCAS.Add(1)
	} else {
		c.eventsCreatedID.Add(1)
	}

	if !success {
		c.eventsFailed.Add(1)
	}

	durationNs := int64(duration)
	c.totalCreationTime.Add(durationNs)

	// Update max creation time
	for {
		current := c.maxCreationTime.Load()
		if durationNs <= current {
			break
		}
		if c.maxCreationTime.CompareAndSwap(current, durationNs) {
			break
		}
	}

	// Increment event type counter
	switch eventType {
	case string(EventTypeObjectCreation):
		c.creationEvents.Add(1)
	case string(EventTypeObjectUpdate):
		c.updateEvents.Add(1)
	case string(EventTypeObjectDeletion):
		c.deleteEvents.Add(1)
	case string(EventTypeBulkOperation):
		c.bulkEvents.Add(1)
	case string(EventTypeSystemConfigChange), string(EventTypeCacheRefresh), string(EventTypeCodeQualityBypass):
		c.systemEvents.Add(1)
	default:
		c.otherEvents.Add(1)
	}

	// Optionally emit event to MetricPipeline (ITEM-912 Phase 2)
	c.emitEvent(map[string]any{
		objects.FieldKeyKind:      KindAuditMetric,
		objects.FieldKeyEventType: eventType,
		FieldKeyUsedCAS:           usedCAS,
		FieldKeyDurationNS:        durationNs,
		FieldKeySuccess:           success,
		FieldKeyTimestamp:         zqktime.NowRFC3339UTC(),
	})
}

// RecordAuditEventValidation records metrics for audit event validation
func (c *AuditMetricsCollector) RecordAuditEventValidation(success bool) {
	if !metricsrecording.Enabled() {
		return
	}
	c.eventsValidated.Add(1)
	if !success {
		c.eventsSkipped.Add(1)
	}

	// Record using new builder-pattern API
	recorder := c.getAuditMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := buildAuditEventValidationMetric(success)
		if err := recorder.Record(MetricNameAuditEventValidation, builder); err != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("%s: %v\n", ErrMsgRecordAuditMetric, err), nil).Log()
		}
	}
}

// RecordAuditEventDuplicate records that a duplicate audit event was detected
func (c *AuditMetricsCollector) RecordAuditEventDuplicate() {
	if !metricsrecording.Enabled() {
		return
	}
	c.eventsDuplicated.Add(1)

	// Record using new builder-pattern API
	recorder := c.getAuditMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := observability.NewBuilder(MetricNameAuditEventDuplicate).
			WithTags(TagAudit, TagEvent, TagDuplicate)
		if err := recorder.Record(MetricNameAuditEventDuplicate, builder); err != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("%s: %v\n", ErrMsgRecordAuditMetric, err), nil).Log()
		}
	}
}

// RecordAuditEventMerged records that a duplicate audit event was successfully merged
func (c *AuditMetricsCollector) RecordAuditEventMerged() {
	if !metricsrecording.Enabled() {
		return
	}
	c.eventsMerged.Add(1)

	// Record using new builder-pattern API
	recorder := c.getAuditMetricsRecorder()
	if recorder != nil && recorder.IsEnabled() {
		builder := observability.NewBuilder(MetricNameAuditEventMerged).
			WithTags(TagAudit, TagEvent, TagMerged)
		if err := recorder.Record(MetricNameAuditEventMerged, builder); err != nil {
			logging.FluentEvent(logging.GetLogger()).Error(fmt.Sprintf("%s: %v\n", ErrMsgRecordAuditMetric, err), nil).Log()
		}
	}
}

// GetSnapshot returns a snapshot of current metrics
func (c *AuditMetricsCollector) GetSnapshot() AuditMetricsSnapshot {
	return AuditMetricsSnapshot{
		EventsCreated:     c.eventsCreated.Load(),
		EventsCreatedCAS:  c.eventsCreatedCAS.Load(),
		EventsCreatedID:   c.eventsCreatedID.Load(),
		EventsFailed:      c.eventsFailed.Load(),
		EventsValidated:   c.eventsValidated.Load(),
		EventsSkipped:     c.eventsSkipped.Load(),
		EventsDuplicated:  c.eventsDuplicated.Load(),
		EventsMerged:      c.eventsMerged.Load(),
		TotalCreationTime: time.Duration(c.totalCreationTime.Load()),
		MaxCreationTime:   time.Duration(c.maxCreationTime.Load()),
		CreationEvents:    c.creationEvents.Load(),
		UpdateEvents:      c.updateEvents.Load(),
		DeleteEvents:      c.deleteEvents.Load(),
		BulkEvents:        c.bulkEvents.Load(),
		SystemEvents:      c.systemEvents.Load(),
		OtherEvents:       c.otherEvents.Load(),
	}
}

// AuditMetricsSnapshot represents a snapshot of audit metrics
type AuditMetricsSnapshot struct {
	EventsCreated     int64
	EventsCreatedCAS  int64
	EventsCreatedID   int64
	EventsFailed      int64
	EventsValidated   int64
	EventsSkipped     int64
	EventsDuplicated  int64
	EventsMerged      int64
	TotalCreationTime time.Duration
	MaxCreationTime   time.Duration
	CreationEvents    int64
	UpdateEvents      int64
	DeleteEvents      int64
	BulkEvents        int64
	SystemEvents      int64
	OtherEvents       int64
}

// AverageCreationTime returns the average time to create an audit event
func (s AuditMetricsSnapshot) AverageCreationTime() time.Duration {
	if s.EventsCreated == 0 {
		return 0
	}
	return s.TotalCreationTime / time.Duration(s.EventsCreated)
}

// CASUsageRate returns the percentage of events created via CAS
func (s AuditMetricsSnapshot) CASUsageRate() float64 {
	if s.EventsCreated == 0 {
		return 0
	}
	return float64(s.EventsCreatedCAS) / float64(s.EventsCreated) * 100
}

// FailureRate returns the percentage of failed event creations
func (s AuditMetricsSnapshot) FailureRate() float64 {
	if s.EventsCreated == 0 {
		return 0
	}
	return float64(s.EventsFailed) / float64(s.EventsCreated) * 100
}

// ValidationSkipRate returns the percentage of events skipped due to validation failures
func (s AuditMetricsSnapshot) ValidationSkipRate() float64 {
	if s.EventsValidated == 0 {
		return 0
	}
	return float64(s.EventsSkipped) / float64(s.EventsValidated) * 100
}

// CollectMetrics creates an audit_metric object from current metrics snapshot
func (c *AuditMetricsCollector) CollectMetrics(
	ctx context.Context,
	secCtx *pkgctx.SecurityContext,
	windowStart, windowEnd time.Time,
) (string, error) {
	if !metricsrecording.Enabled() {
		return "", nil
	}
	snapshot := c.GetSnapshot()

	// Calculate derived metrics
	avgCreationTime := snapshot.AverageCreationTime()
	casUsageRate := snapshot.CASUsageRate()
	failureRate := snapshot.FailureRate()
	validationSkipRate := snapshot.ValidationSkipRate()

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
	var metricID string
	if fileStorage, ok := c.storage.(*FileObjectStorage); ok {
		// Check if CAS is enabled (CAS uses hash-based filenames, so sequence-based IDs don't work)
		if fileStorage.usesContentAddressableStorage(objects.KindCommandMetric) {
			// Use timestamp-based ID for CAS-enabled kinds (ensures uniqueness)
			metricID = fmt.Sprintf("%s%d", PrefixCommand, now.UnixNano())
		} else {
			// Use thread-safe batch generator for non-CAS kinds (consistent with audit IDs)
			generatedID, err := fileStorage.generateID(ctx, objects.KindCommandMetric)
			if err != nil {
				// Fallback to timestamp-based ID if generation fails
				metricID = fmt.Sprintf("%s%d", PrefixCommand, now.UnixNano())
			} else {
				metricID = generatedID
			}
		}
	} else {
		// For graph storage or other backends, use timestamp-based ID
		metricID = fmt.Sprintf("%s%d", PrefixCommand, now.UnixNano())
	}

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
	builder.Command(ValueAuditEventCollection). // Placeholder command name
							NormalizedCmd(ValueAuditEventCollection).
							InvocationCount(int(snapshot.EventsCreated)).
							SuccessCount(int(snapshot.EventsCreated - snapshot.EventsFailed)).
							FailureCount(int(snapshot.EventsFailed)).
							AvgDurationSeconds(avgCreationTime.Seconds())

	// Set required fields
	if snapshot.MaxCreationTime > 0 {
		builder.SlowestDurationSeconds(snapshot.MaxCreationTime.Seconds())
		builder.FastestDurationSeconds(avgCreationTime.Seconds()) // Use avg as fastest approximation
	} else {
		builder.SlowestDurationSeconds(avgCreationTime.Seconds())
		builder.FastestDurationSeconds(avgCreationTime.Seconds())
	}
	builder.BaselineDurationSeconds(avgCreationTime.Seconds())

	// Calculate rates
	var errorRate float64
	if snapshot.EventsCreated > 0 {
		errorRate = float64(snapshot.EventsFailed) / float64(snapshot.EventsCreated) * 100
	}
	builder.ErrorRate(errorRate).
		TimeoutRate(0.0). // Audit events don't have timeouts
		TimeoutCount(0)   // Audit events don't have timeouts

	// Build the instance
	metricObj, err := builder.Build()
	if err != nil {
		logger := logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
		StorageLog(logger).Warn(LogEventStorageMetricsCommandBuildFailed).
			WithError(err).
			Log()
		return "", errfmt.Newf(ErrMsgBuildCommandMetric).Wrap(err)
	}

	// Add audit-specific fields as metadata (since command_metric doesn't have these fields)
	// These are stored as additional fields that will be preserved but not validated
	var metadata map[string]any
	if metadataVal, ok := metricObj[auditCmdMetricObjKeyMetadata].(map[string]any); ok && metadataVal != nil {
		metadata = metadataVal
	} else {
		// Create metadata map if it doesn't exist
		metadata = make(map[string]any)
		metricObj[auditCmdMetricObjKeyMetadata] = metadata
	}

	if metadata != nil {
		metadata[auditCmdMetricMetaEventsCreatedCAS] = snapshot.EventsCreatedCAS
		metadata[auditCmdMetricMetaEventsCreatedID] = snapshot.EventsCreatedID
		metadata[auditCmdMetricMetaEventsValidated] = snapshot.EventsValidated
		metadata[auditCmdMetricMetaEventsSkipped] = snapshot.EventsSkipped
		metadata[auditCmdMetricMetaEventsDuplicated] = snapshot.EventsDuplicated
		metadata[auditCmdMetricMetaEventsMerged] = snapshot.EventsMerged
		metadata[auditCmdMetricMetaMaxCreationTimeMs] = snapshot.MaxCreationTime.Milliseconds()
		metadata[auditCmdMetricMetaCasUsageRate] = casUsageRate
		metadata[auditCmdMetricMetaFailureRate] = failureRate
		metadata[auditCmdMetricMetaValidationSkipRate] = validationSkipRate
		metadata[auditCmdMetricMetaCreationEvents] = snapshot.CreationEvents
		metadata[auditCmdMetricMetaUpdateEvents] = snapshot.UpdateEvents
		metadata[auditCmdMetricMetaDeleteEvents] = snapshot.DeleteEvents
		metadata[auditCmdMetricMetaBulkEvents] = snapshot.BulkEvents
		metadata[auditCmdMetricMetaSystemEvents] = snapshot.SystemEvents
		metadata[auditCmdMetricMetaOtherEvents] = snapshot.OtherEvents
		metadata[auditCmdMetricMetaMeasurementPeriod] = windowEnd.Sub(windowStart).Seconds()
	} else {
		// Create metadata map if it doesn't exist
		metricObj[auditCmdMetricObjKeyMetadata] = map[string]any{
			auditCmdMetricMetaEventsCreatedCAS:   snapshot.EventsCreatedCAS,
			auditCmdMetricMetaEventsCreatedID:    snapshot.EventsCreatedID,
			auditCmdMetricMetaEventsValidated:    snapshot.EventsValidated,
			auditCmdMetricMetaEventsSkipped:      snapshot.EventsSkipped,
			auditCmdMetricMetaEventsDuplicated:   snapshot.EventsDuplicated,
			auditCmdMetricMetaEventsMerged:       snapshot.EventsMerged,
			auditCmdMetricMetaMaxCreationTimeMs:  snapshot.MaxCreationTime.Milliseconds(),
			auditCmdMetricMetaCasUsageRate:       casUsageRate,
			auditCmdMetricMetaFailureRate:        failureRate,
			auditCmdMetricMetaValidationSkipRate: validationSkipRate,
			auditCmdMetricMetaCreationEvents:     snapshot.CreationEvents,
			auditCmdMetricMetaUpdateEvents:       snapshot.UpdateEvents,
			auditCmdMetricMetaDeleteEvents:       snapshot.DeleteEvents,
			auditCmdMetricMetaBulkEvents:         snapshot.BulkEvents,
			auditCmdMetricMetaSystemEvents:       snapshot.SystemEvents,
			auditCmdMetricMetaOtherEvents:        snapshot.OtherEvents,
			auditCmdMetricMetaMeasurementPeriod:  windowEnd.Sub(windowStart).Seconds(),
		}
	}

	if c.storage == nil {
		return "", errfmt.Errorf(ErrMsgStorageNotConfigured)
	}

	// Create metric asynchronously (non-blocking) using goroutine pattern
	// This ensures metrics creation never blocks the calling goroutine
	metricIDChan := make(chan string, 1)
	errChan := make(chan error, 1)

	goroutinelabels.NewGoroutine(OpNameAuditMetricsCollectorAsync, DescAuditMetricsAsync).
		StartWithContext(ctx, func(ctx context.Context) error {
			if err := c.storage.Create(ctx, secCtx, metricObj); err != nil {
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

	// Wait for result with timeout
	select {
	case id := <-metricIDChan:
		return id, nil
	case err := <-errChan:
		return "", err
	case <-time.After(DefaultAuditMetricTimeout):
		return "", errfmt.Errorf(ErrMsgMetricTimeout, DefaultAuditMetricTimeout)
	case <-ctx.Done():
		return "", ctx.Err()
	}
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
