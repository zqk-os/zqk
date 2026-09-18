package coordination

import (
	"context"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
	"github.com/zqk-os/zqk/pkg/objects"
)

// Field keys for logging, audit metadata, and metrics (consistent with diagnostics.jsonl and consumers).
const (
	keyOperationID     = "operation_id"
	keyOperationType   = "operation_type"
	keyProgress        = "progress"
	keyTotal           = "total"
	keyPercentComplete = "percent_complete"
	keyMessage         = "message"
	keyEventType       = "event_type"
	keyOperation       = "operation"
	keySeverity        = "severity"
	keyTargetKind      = "target_kind"
	keyOldStatus       = "old_status"
	keyNewStatus       = "new_status"
	keyDuration        = "duration"
	keyError           = "error"
)

// Audit event_type value used for progress/status events (valid enum from audit_event spec).
const auditEventTypeSystemConfigChange = "system_config_change"

// ProgressHelper provides a unified interface for emitting progress events through the coordinator
// This enables progress-based coordination across all systems.
// EmitStatusChange deduplicates by (oldStatus, newStatus) so repeated heartbeats with the same
// transition (e.g. "started" -> "in_progress") do not produce duplicate events and inflate metrics.
type ProgressHelper struct {
	coordinator   *Coordinator
	projectRoot   string
	operationID   string
	operationType string
	profile       string // CLI context profile for logging format

	// statusChangeMu guards lastEmittedStatus so we only emit a status-change event once per transition
	statusChangeMu       sync.Mutex
	lastEmittedOldStatus string
	lastEmittedNewStatus string
}

// NewProgressHelper creates a new progress helper for a specific operation
func NewProgressHelper(
	coordinator *Coordinator,
	projectRoot string,
	operationID string,
	operationType string,
	profile string,
) *ProgressHelper {
	return &ProgressHelper{
		coordinator:   coordinator,
		projectRoot:   projectRoot,
		operationID:   operationID,
		operationType: operationType,
		profile:       profile,
	}
}

// EmitProgress emits a progress update event
// This routes progress updates to logging, metrics, and operational channels
// Audit events are only emitted for progress-summary checkpoints (configurable thresholds)
func (h *ProgressHelper) EmitProgress(
	ctx context.Context,
	progress int,
	total int,
	message string,
	fields map[string]any,
	emitAudit bool, // Whether to emit audit event (for milestones)
) error {
	if h.coordinator == nil {
		return nil // Best effort - skip if no coordinator
	}

	// Calculate percentage safely without division by zero
	percent := 0.0
	if total > 0 {
		percent = float64(progress) / float64(total) * 100
		if percent > 100 {
			percent = 100
		}
	}

	// Build logging fields
	loggingFields := []LoggingField{
		{Key: keyOperationID, Value: h.operationID},
		{Key: keyOperationType, Value: h.operationType},
		{Key: keyProgress, Value: progress},
		{Key: keyTotal, Value: total},
		{Key: keyPercentComplete, Value: percent},
	}
	if message != emptyValue {
		loggingFields = append(loggingFields, LoggingField{Key: keyMessage, Value: message})
	}
	// Add custom fields
	for k, v := range fields {
		loggingFields = append(loggingFields, LoggingField{Key: k, Value: v})
	}

	// Build audit metadata (only if emitAudit is true)
	var auditMetadata map[string]any
	if emitAudit {
		auditMetadata = make(map[string]any)
		auditMetadata[keyEventType] = auditEventTypeSystemConfigChange
		auditMetadata[keyOperation] = fmt.Sprintf("%s progress: %d/%d (%.1f%%)", h.operationType, progress, total, percent)
		auditMetadata[keySeverity] = "low"
		auditMetadata[keyTargetKind] = h.operationType
		auditMetadata[keyProgress] = progress
		auditMetadata[keyTotal] = total
		auditMetadata[keyPercentComplete] = percent
		maps.Copy(auditMetadata, fields)
	}

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[keyOperation] = h.operationType
	metricsData[keyEventType] = "progress"
	metricsData[keyProgress] = progress
	metricsData[keyTotal] = total
	metricsData[keyPercentComplete] = percent
	maps.Copy(metricsData, fields)

	// Create event data
	eventData := &EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Determine status
	status := "in_progress"
	if total > 0 && progress >= total {
		status = "complete"
	}

	// Create event context
	eventCtx := NewEventContext(h.operationID, h.operationType, status).
		WithEventData(eventData).
		WithContext(ctx)

	// Enable channels: logging + metrics + operational always, audit only if emitAudit
	eventCtx = eventCtx.WithChannels(true, emitAudit, true, true)

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine("progress_helper_emitter", "emitting progress event").
		StartSimple(func() {
			_ = h.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})

	return nil
}

// EmitProgressSummary emits a sparse progress-summary checkpoint (e.g. 25%, 50%).
// Do not call this "milestone" — that is objects.KindMilestone (MIL-*).
// TRACK: BLI-CEF-LOG-SIGNAL-VS-NOISE-001
func (h *ProgressHelper) EmitProgressSummary(
	ctx context.Context,
	threshold string, // e.g., "25%", "50%", "75%", "complete"
	progress int,
	total int,
	message string,
	fields map[string]any,
) error {
	return h.EmitProgress(ctx, progress, total, fmt.Sprintf("Progress summary: %s - %s", threshold, message), fields, true)
}

// EmitStatusChange emits a status change event.
// Deduplicates: only one event per (oldStatus, newStatus) transition per operation,
// so heartbeat callers that repeatedly emit "started" -> "in_progress" do not inflate event counts.
func (h *ProgressHelper) EmitStatusChange(
	ctx context.Context,
	oldStatus string,
	newStatus string,
	message string,
	fields map[string]any,
) error {
	if h.coordinator == nil {
		return nil // Best effort
	}

	var shouldEmit bool
	_ = concurrency.RunInLock(&h.statusChangeMu, func() error {
		// Allow repeated emissions when oldStatus == newStatus (heartbeat) so the user sees routine progress
		if oldStatus == newStatus {
			shouldEmit = true
			return nil
		}
		if h.lastEmittedOldStatus == oldStatus && h.lastEmittedNewStatus == newStatus {
			shouldEmit = false
			return nil // Already emitted this transition; avoid duplicate events
		}
		h.lastEmittedOldStatus = oldStatus
		h.lastEmittedNewStatus = newStatus
		shouldEmit = true
		return nil
	})
	if !shouldEmit {
		return nil
	}

	// Build logging fields
	loggingFields := []LoggingField{
		{Key: keyOperationID, Value: h.operationID},
		{Key: keyOperationType, Value: h.operationType},
		{Key: keyOldStatus, Value: oldStatus},
		{Key: keyNewStatus, Value: newStatus},
	}
	if message != emptyValue {
		loggingFields = append(loggingFields, LoggingField{Key: keyMessage, Value: message})
	}
	// Add custom fields
	for k, v := range fields {
		loggingFields = append(loggingFields, LoggingField{Key: k, Value: v})
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[keyEventType] = auditEventTypeSystemConfigChange
	auditMetadata[keyOperation] = fmt.Sprintf("%s status change: %s -> %s", h.operationType, oldStatus, newStatus)
	auditMetadata[keySeverity] = "low"
	auditMetadata[keyTargetKind] = h.operationType
	auditMetadata[keyOldStatus] = oldStatus
	auditMetadata[keyNewStatus] = newStatus
	maps.Copy(auditMetadata, fields)

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[keyOperation] = h.operationType
	metricsData[keyEventType] = "status_change"
	metricsData[keyOldStatus] = oldStatus
	metricsData[keyNewStatus] = newStatus
	maps.Copy(metricsData, fields)

	// Create event data
	eventData := &EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Determine status based on new status
	status := newStatus
	switch newStatus {
	case objects.ObjectStatusError, objects.ObjectStatusFailed:
		status = OperationStatusError
	case OperationStatusComplete, objects.ObjectStatusCompleted:
		status = OperationStatusComplete
	}

	// Create event context
	eventCtx := NewEventContext(h.operationID, h.operationType, status).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, true, true) // All channels for status changes

	if lvl, ok := fields["level"].(string); ok && lvl != emptyValue {
		eventCtx.WithLevel(lvl)
	} else if lvl, ok := fields["log_level"].(string); ok && lvl != emptyValue {
		eventCtx.WithLevel(lvl)
	}

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine("progress_helper_emitter", "emitting progress event").
		StartSimple(func() {
			_ = h.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})

	return nil
}

// EmitError emits an error event
func (h *ProgressHelper) EmitError(
	ctx context.Context,
	err error,
	message string,
	fields map[string]any,
) error {
	if h.coordinator == nil {
		return nil // Best effort
	}

	// Build logging fields (include old_status/new_status for consistent diagnostics.jsonl)
	loggingFields := []LoggingField{
		{Key: keyOperationID, Value: h.operationID},
		{Key: keyOperationType, Value: h.operationType},
		{Key: keyOldStatus, Value: objects.ObjectStatusInProgress},
		{Key: keyNewStatus, Value: objects.ObjectStatusError},
	}
	if message != emptyValue {
		loggingFields = append(loggingFields, LoggingField{Key: keyMessage, Value: message})
	}
	if err != nil {
		loggingFields = append(loggingFields, LoggingField{Key: keyError, Value: err.Error()})
	}
	// Add custom fields
	for k, v := range fields {
		loggingFields = append(loggingFields, LoggingField{Key: k, Value: v})
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[keyEventType] = auditEventTypeSystemConfigChange
	if message != emptyValue {
		auditMetadata[keyOperation] = fmt.Sprintf("%s error: %s", h.operationType, message)
	} else {
		auditMetadata[keyOperation] = fmt.Sprintf("%s error", h.operationType)
	}
	auditMetadata[keySeverity] = "high"
	auditMetadata[keyTargetKind] = h.operationType
	auditMetadata[keyOldStatus] = "in_progress"
	auditMetadata[keyNewStatus] = "error"
	if err != nil {
		auditMetadata[keyError] = err.Error()
	}
	maps.Copy(auditMetadata, fields)

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[keyOperation] = h.operationType
	metricsData[keyEventType] = "error"
	metricsData[keyOldStatus] = "in_progress"
	metricsData[keyNewStatus] = "error"
	if err != nil {
		metricsData[keyError] = err.Error()
	}
	maps.Copy(metricsData, fields)

	// Create event data
	eventData := &EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Create event context with error
	eventCtx := NewEventContext(h.operationID, h.operationType, "error").
		WithEventData(eventData).
		WithContext(ctx).
		WithError(err).
		WithChannels(true, true, true, true) // All channels for errors

	// Emit synchronously so event is written before process exits (coordinator logs terminal events sync)
	_ = h.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Best-effort

	return nil
}

// EmitCompletion emits a completion event
func (h *ProgressHelper) EmitCompletion(
	ctx context.Context,
	duration time.Duration,
	message string,
	fields map[string]any,
) error {
	if h.coordinator == nil {
		return nil // Best effort
	}

	// Build logging fields (include old_status/new_status for consistent diagnostics.jsonl)
	loggingFields := []LoggingField{
		{Key: keyOperationID, Value: h.operationID},
		{Key: keyOperationType, Value: h.operationType},
		{Key: keyOldStatus, Value: "in_progress"},
		{Key: keyNewStatus, Value: "complete"},
		{Key: keyDuration, Value: duration.String()},
	}
	if message != emptyValue {
		loggingFields = append(loggingFields, LoggingField{Key: keyMessage, Value: message})
	}
	// Add custom fields
	for k, v := range fields {
		loggingFields = append(loggingFields, LoggingField{Key: k, Value: v})
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[keyEventType] = auditEventTypeSystemConfigChange
	if message != emptyValue {
		auditMetadata[keyOperation] = fmt.Sprintf("%s completed: %s", h.operationType, message)
	} else {
		auditMetadata[keyOperation] = fmt.Sprintf("%s completed", h.operationType)
	}
	auditMetadata[keySeverity] = "medium"
	auditMetadata[keyTargetKind] = h.operationType
	auditMetadata[keyOldStatus] = "in_progress"
	auditMetadata[keyNewStatus] = "complete"
	auditMetadata[keyDuration] = duration.String()
	maps.Copy(auditMetadata, fields)

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[keyOperation] = h.operationType
	metricsData[keyEventType] = "completion"
	metricsData[keyOldStatus] = "in_progress"
	metricsData[keyNewStatus] = "complete"
	metricsData[keyDuration] = duration.Seconds()
	maps.Copy(metricsData, fields)

	// Create event data
	eventData := &EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Create event context
	eventCtx := NewEventContext(h.operationID, h.operationType, "complete").
		WithEventData(eventData).
		WithContext(ctx).
		WithDuration(duration).
		WithChannels(true, true, true, true) // All channels for completion

	// Emit synchronously so event is written before process exits (coordinator logs terminal events sync)
	_ = h.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Best-effort

	return nil
}
