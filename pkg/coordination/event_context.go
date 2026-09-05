package coordination

import (
	"context"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"
)

const (
	// Coordination operation-status values (not object lifecycle statuses).
	OperationStatusComplete      = "complete"
	OperationStatusDependencyRef = "dependency_ref"
	OperationStatusError         = "error"
	OperationStatusProgress      = "progress"
	OperationStatusStart         = "start"
)

// EventContext carries rich context about an operation/event
// This context drives multi-channel emission and coordination decisions
type EventContext struct {
	// Operation metadata
	OperationID   string // Unique identifier for this operation
	OperationType string // Type of operation (e.g., "snapshot_expand", "migration", "validation")
	Status        string // Operation status: "start", "progress", "complete", "error"

	// Timing
	Timestamp time.Time
	Duration  time.Duration // Duration of operation (for completion events)

	// Event data (shared across channels)
	// This can be populated from buildSnapshotExpandEventData or similar builders
	EventData *EventData

	// Coordination hints for sequencing
	Dependencies  []string // Operation IDs this operation depends on
	Triggers      []string // Operation IDs/events this operation triggers
	CorrelationID string   // For tracing events across operations

	// Routing decisions (which channels to emit to)
	EmitLogging     bool
	EmitAudit       bool
	EmitMetrics     bool
	EmitOperational bool

	// Error information (if status is "error")
	Error error

	// Go context for cancellation/timeout
	Ctx context.Context
}

// EventData contains structured data for different channels
// This is the output from event data builders (like buildSnapshotExpandEventData)
type EventData struct {
	// Logging fields (structured logging)
	LoggingFields []LoggingField

	// Audit metadata (for audit_event objects)
	AuditMetadata map[string]any

	// Metrics data (for base_metric objects)
	MetricsData map[string]any
}

// LoggingField represents a structured logging field
// Compatible with pkg/logging.Field
type LoggingField struct {
	Key   string
	Value any
}

// NewEventContext creates a new EventContext with sensible defaults
func NewEventContext(operationID, operationType, status string) *EventContext {
	return &EventContext{
		OperationID:     operationID,
		OperationType:   operationType,
		Status:          status,
		Timestamp:       time.Now().UTC(),
		EventData:       &EventData{},
		EmitLogging:     true,
		EmitAudit:       true,
		EmitMetrics:     true,
		EmitOperational: true,
		Ctx:             pkgctx.NewSystemContext(),
	}
}

// WithEventData sets the event data for this context
func (ec *EventContext) WithEventData(data *EventData) *EventContext {
	ec.EventData = data
	return ec
}

// WithError sets the error for this context (and sets status to "error" if not already set)
func (ec *EventContext) WithError(err error) *EventContext {
	ec.Error = err
	if ec.Status != OperationStatusError {
		ec.Status = OperationStatusError
	}
	return ec
}

// WithDuration sets the duration for this context
func (ec *EventContext) WithDuration(duration time.Duration) *EventContext {
	ec.Duration = duration
	return ec
}

// WithDependencies sets the dependencies for this context
func (ec *EventContext) WithDependencies(deps []string) *EventContext {
	ec.Dependencies = deps
	return ec
}

// WithTriggers sets the triggers for this context
func (ec *EventContext) WithTriggers(triggers []string) *EventContext {
	ec.Triggers = triggers
	return ec
}

// WithCorrelationID sets the correlation ID for this context
func (ec *EventContext) WithCorrelationID(correlationID string) *EventContext {
	ec.CorrelationID = correlationID
	return ec
}

// WithContext sets the Go context for this event context
func (ec *EventContext) WithContext(ctx context.Context) *EventContext {
	ec.Ctx = ctx
	return ec
}

// WithChannels controls which channels to emit to
func (ec *EventContext) WithChannels(logging, audit, metrics, operational bool) *EventContext {
	ec.EmitLogging = logging
	ec.EmitAudit = audit
	ec.EmitMetrics = metrics
	ec.EmitOperational = operational
	return ec
}
