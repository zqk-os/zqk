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
func (h *ErrorHelper) emitEventInternal(
	ctx context.Context,
	eventType string,
	err error,
	message string,
	fields map[string]any,
	severity string,
	opDesc string,
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
	for k, v := range fields {
		loggingFields = append(loggingFields, LoggingField{Key: k, Value: v})
	}

	// Build audit metadata
	auditMetadata := make(map[string]any)
	auditMetadata[logKeyEventType] = eventTypeSystemConfigChange
	auditMetadata[logKeyOperation] = opDesc
	auditMetadata[logKeySeverity] = severity
	auditMetadata[logKeyTargetKind] = h.operationType
	if err != nil {
		auditMetadata[logKeyError] = err.Error()
	}
	maps.Copy(auditMetadata, fields)

	// Build metrics data
	metricsData := make(map[string]any)
	metricsData[logKeyOperation] = h.operationType
	metricsData[logKeyEventType] = string(eventType)
	metricsData[logKeySeverity] = severity
	if err != nil {
		metricsData[logKeyError] = err.Error()
	}
	maps.Copy(metricsData, fields)

	eventData := &EventData{
		LoggingFields: loggingFields,
		AuditMetadata: auditMetadata,
		MetricsData:   metricsData,
	}

	eventCtx := NewEventContext(h.operationID, h.operationType, eventType).
		WithEventData(eventData).
		WithContext(ctx).
		WithError(err).
		WithChannels(true, true, true, true)

	goroutinelabels.NewGoroutine(goroutineNameErrorEmitter, goroutinePurposeEmitError).
		StartSimple(func() {
			_ = h.coordinator.Emit(ctx, eventCtx) //nolint:errcheck // Async, best-effort
		})

	return nil
}

func (h *ErrorHelper) EmitError(
	ctx context.Context,
	err error,
	message string,
	fields map[string]any,
	severity string, // severityLow, severityMedium, severityHigh, severityCritical
) error {
	if severity == emptyErrValue {
		severity = severityHigh // Default to high for errors
	}
	opDesc := fmt.Sprintf(operationErrorOnlyFmt, h.operationType)
	if message != emptyErrValue {
		opDesc = fmt.Sprintf(operationErrorFmt, h.operationType, message)
	}
	return h.emitEventInternal(ctx, eventTypeError, err, message, fields, severity, opDesc)
}

// EmitWarning emits a warning event through the coordinator
func (h *ErrorHelper) EmitWarning(
	ctx context.Context,
	message string,
	fields map[string]any,
) error {
	opDesc := fmt.Sprintf(operationWarningFmt, h.operationType, message)
	return h.emitEventInternal(ctx, eventTypeWarning, nil, message, fields, severityMedium, opDesc)
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
