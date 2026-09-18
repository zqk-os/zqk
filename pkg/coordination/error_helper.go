package coordination

import (
	"context"
	"fmt"
	"maps"
	"time"

	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/goroutinelabels"
)

const (
	logKeyOperationID    = "operation_id"
	logKeyOperationType  = "operation_type"
	logKeyMessage        = "message"
	logKeyError          = "error"
	logKeyRetryCount     = "retry_count"
	logKeyMaxRetries     = "max_retries"
	logKeyRetryable      = "retryable"
	logKeyTimeoutDur     = "timeout_duration"
	logKeyTimeoutSeconds = "timeout_seconds"
	logKeyEventType      = "event_type"
	logKeyOperation      = "operation"
	logKeySeverity       = "severity"
	logKeyTargetKind     = "target_kind"

	eventTypeSystemConfigChange = "system_config_change"
	eventTypeError              = "error"
	eventTypeWarning            = "warning"
	operationErrorFmt           = "%s error: %s"
	operationErrorOnlyFmt       = "%s error"
	operationWarningFmt         = "%s warning: %s"
	timeoutErrorFmt             = "operation timed out after %v"

	severityLow      = "low"
	severityMedium   = "medium"
	severityHigh     = "high"
	severityCritical = "critical"

	goroutineNameErrorEmitter = "error_helper_emitter"
	goroutinePurposeEmitError = "emitting error event"
	emptyErrValue             = ""
)

// ErrorHelper provides a unified interface for emitting error events through the coordinator
// This enables error-based coordination across all systems
type ErrorHelper struct {
	coordinator   *Coordinator
	projectRoot   string
	operationID   string
	operationType string
	profile       string // CLI context profile for logging format
}

// NewErrorHelper creates a new error helper for a specific operation
func NewErrorHelper(
	coordinator *Coordinator,
	projectRoot string,
	operationID string,
	operationType string,
	profile string,
) *ErrorHelper {
	return &ErrorHelper{
		coordinator:   coordinator,
		projectRoot:   projectRoot,
		operationID:   operationID,
		operationType: operationType,
		profile:       profile,
	}
}

// EmitError emits an error event through the coordinator
// This routes errors to logging, audit, metrics, and operational channels
func (h *ErrorHelper) EmitError(
	ctx context.Context,
	err error,
	message string,
	fields map[string]any,
	severity string, // severityLow, severityMedium, severityHigh, severityCritical
) error {
	if h.coordinator == nil {
		return nil // Best effort - skip if no coordinator
	}

	// Build logging fields
	loggingFields := []LoggingField{
		{Key: logKeyOperationID, Value: h.operationID},
		{Key: logKeyOperationType, Value: h.operationType},
	}
	if message != emptyErrValue {
		loggingFields = append(loggingFields, LoggingField{Key: logKeyMessage, Value: message})
	}
	if err != nil {
		loggingFields = append(loggingFields, LoggingField{Key: logKeyError, Value: err.Error()})
	}
	// Add custom fields
	for k, v := range fields {
		loggingFields = append(loggingFields, LoggingField{Key: k, Value: v})
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[logKeyEventType] = eventTypeSystemConfigChange // Use valid enum value
	if message != emptyErrValue {
		auditMetadata[logKeyOperation] = fmt.Sprintf(operationErrorFmt, h.operationType, message)
	} else {
		auditMetadata[logKeyOperation] = fmt.Sprintf(operationErrorOnlyFmt, h.operationType)
	}
	if severity == emptyErrValue {
		severity = severityHigh // Default to high for errors
	}
	auditMetadata[logKeySeverity] = severity
	auditMetadata[logKeyTargetKind] = h.operationType
	if err != nil {
		auditMetadata[logKeyError] = err.Error()
	}
	// Add custom fields to audit metadata
	maps.Copy(auditMetadata, fields)

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[logKeyOperation] = h.operationType
	metricsData[logKeyEventType] = eventTypeError
	metricsData[logKeySeverity] = severity
	if err != nil {
		metricsData[logKeyError] = err.Error()
	}
	// Add custom fields to metrics
	maps.Copy(metricsData, fields)

	// Create event data
	eventData := &EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Create event context with error
	eventCtx := NewEventContext(h.operationID, h.operationType, eventTypeError).
		WithEventData(eventData).
		WithContext(ctx).
		WithError(err).
		WithChannels(true, true, true, true) // All channels for errors

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine(goroutineNameErrorEmitter, goroutinePurposeEmitError).
		StartSimple(func() {
			_ = h.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})

	return nil
}

// EmitWarning emits a warning event through the coordinator
func (h *ErrorHelper) EmitWarning(
	ctx context.Context,
	message string,
	fields map[string]any,
) error {
	if h.coordinator == nil {
		return nil // Best effort
	}

	// Build logging fields
	loggingFields := []LoggingField{
		{Key: logKeyOperationID, Value: h.operationID},
		{Key: logKeyOperationType, Value: h.operationType},
	}
	if message != emptyErrValue {
		loggingFields = append(loggingFields, LoggingField{Key: logKeyMessage, Value: message})
	}
	// Add custom fields
	for k, v := range fields {
		loggingFields = append(loggingFields, LoggingField{Key: k, Value: v})
	}

	// Build audit metadata (warnings are lower severity)
	auditMetadata := make(map[string]any)
	auditMetadata[logKeyEventType] = eventTypeSystemConfigChange
	auditMetadata[logKeyOperation] = fmt.Sprintf(operationWarningFmt, h.operationType, message)
	auditMetadata[logKeySeverity] = severityMedium
	auditMetadata[logKeyTargetKind] = h.operationType
	// Add custom fields
	maps.Copy(auditMetadata, fields)

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[logKeyOperation] = h.operationType
	metricsData[logKeyEventType] = eventTypeWarning
	// Add custom fields
	maps.Copy(metricsData, fields)

	// Create event data
	eventData := &EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	// Create event context
	eventCtx := NewEventContext(h.operationID, h.operationType, eventTypeWarning).
		WithEventData(eventData).
		WithContext(ctx).
		WithChannels(true, true, true, true) // All channels for warnings

	// Emit via coordinator (async, non-blocking)
	goroutinelabels.NewGoroutine(goroutineNameErrorEmitter, goroutinePurposeEmitError).
		StartSimple(func() {
			_ = h.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})

	return nil
}

// EmitRetryableError emits an error event with retry information
func (h *ErrorHelper) EmitRetryableError(
	ctx context.Context,
	err error,
	message string,
	retryCount, maxRetries int,
	fields map[string]any,
) error {
	if fields == nil {
		fields = make(map[string]any)
	}
	fields[logKeyRetryCount] = retryCount
	fields[logKeyMaxRetries] = maxRetries
	fields[logKeyRetryable] = true

	severity := severityMedium
	if retryCount >= maxRetries {
		severity = severityHigh // Final failure is high severity
	}

	return h.EmitError(ctx, err, message, fields, severity)
}

// EmitTimeoutError emits a timeout error event
func (h *ErrorHelper) EmitTimeoutError(
	ctx context.Context,
	timeoutDuration time.Duration,
	message string,
	fields map[string]any,
) error {
	if fields == nil {
		fields = make(map[string]any)
	}
	fields[logKeyTimeoutDur] = timeoutDuration.String()
	fields[logKeyTimeoutSeconds] = timeoutDuration.Seconds()

	err := errfmt.Errorf(timeoutErrorFmt, timeoutDuration)
	return h.EmitError(ctx, err, message, fields, severityHigh)
}
