package storage

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/metricsrecording"
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/observability"
	"github.com/zqk-os/zqk/pkg/storage/audit"
	"github.com/zqk-os/zqk/pkg/zqktime"
)

// AuditMetricsCollector collects metrics for audit event operations
type AuditMetricsCollector struct {
	storage ObjectStorageProvider

	// Event emission (BLI-912 Phase 2: MetricPipeline integration)
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

// SetEventEmitter sets the event emitter function for MetricPipeline integration (BLI-912 Phase 2)
// Events are emitted asynchronously to avoid blocking audit event creation
func (c *AuditMetricsCollector) SetEventEmitter(emitter func(event map[string]any) error, enabled bool) {
	c.eventEmitter = emitter
	c.emitEvents.Store(enabled)
}

// emitEvent optionally emits an event to the metrics pipeline (BLI-912 Phase 2)
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
	switch audit.ClassifyEventType(eventType) {
	case audit.BucketCreation:
		c.creationEvents.Add(1)
	case audit.BucketUpdate:
		c.updateEvents.Add(1)
	case audit.BucketDelete:
		c.deleteEvents.Add(1)
	case audit.BucketBulk:
		c.bulkEvents.Add(1)
	case audit.BucketSystem:
		c.systemEvents.Add(1)
	default:
		c.otherEvents.Add(1)
	}

	// Optionally emit event to MetricPipeline (BLI-912 Phase 2)
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

// AuditMetricsSnapshot is the storage alias for audit.MetricsSnapshot.
type AuditMetricsSnapshot = audit.MetricsSnapshot

var _ audit.CreationRecorder = (*AuditMetricsCollector)(nil)

// CollectMetrics lives in audit_metrics_collect.go.
