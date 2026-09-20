package coordination

import "time"

// OperationTypeProjectRootChanged is the operation type when the persisted current project root
// is set or changed (e.g. after zqk use). Subscribers can filter on event.OperationType == OperationTypeProjectRootChanged.
const OperationTypeProjectRootChanged = "project_root_changed"

// EventTypeLifecycleDependencyRef is the operational event type for one-level dependency propagation.
// Emitted when an object's status toggles; subscribers re-evaluate refs/dependents and apply status rules.
const EventTypeLifecycleDependencyRef = "lifecycle.dependency_ref"

// OperationalEventSubscriber is an interface for subscribers that listen to operational events
// for process coordination and sequencing
type OperationalEventSubscriber interface {
	// ID returns a unique identifier for this subscriber
	ID() string

	// HandleEvent processes an operational event
	// This is where coordination logic lives (e.g., trigger dependent operations)
	HandleEvent(event *OperationalEvent) error

	// EventTypes returns the event types this subscriber is interested in
	// Empty slice means subscribe to all events
	EventTypes() []string

	// IsActive returns whether the subscriber is still active
	// Inactive subscribers will be automatically unsubscribed
	IsActive() bool
}

// OperationalEvent represents an event for process coordination
// These events enable subscribers to coordinate and sequence operations
type OperationalEvent struct {
	// Event type: "operation.start", "operation.complete", "operation.error", "operation.progress"
	Type string

	// Operation metadata
	OperationID   string
	OperationType string
	Status        string

	// Timing
	Timestamp time.Time
	Duration  time.Duration

	// Event metadata
	Metadata map[string]any

	// Coordination hints
	Dependencies  []string // Operation IDs this operation depends on
	Triggers      []string // Operation IDs/events this operation triggers
	CorrelationID string   // For tracing events across operations

	// Error information (if applicable)
	Error string
}

// NewOperationalEvent creates a new operational event from an EventContext
func NewOperationalEvent(eventCtx *EventContext) *OperationalEvent {
	event := &OperationalEvent{
		Type:          buildOperationalEventType(eventCtx.Status),
		OperationID:   eventCtx.OperationID,
		OperationType: eventCtx.OperationType,
		Status:        eventCtx.Status,
		Timestamp:     eventCtx.Timestamp,
		Duration:      eventCtx.Duration,
		Dependencies:  eventCtx.Dependencies,
		Triggers:      eventCtx.Triggers,
		CorrelationID: eventCtx.CorrelationID,
		Metadata:      make(map[string]any),
	}

	// Include metrics data in metadata for subscribers
	if eventCtx.EventData != nil && eventCtx.EventData.MetricsData != nil {
		for k, v := range eventCtx.EventData.MetricsData {
			event.Metadata[k] = v
		}
	}

	// Include logging fields in metadata for subscribers (so they can access cache operation details, etc.)
	if eventCtx.EventData != nil && eventCtx.EventData.LoggingFields != nil {
		for _, field := range eventCtx.EventData.LoggingFields {
			event.Metadata[field.Key] = field.Value
		}
	}

	if eventCtx.Error != nil {
		event.Error = eventCtx.Error.Error()
		event.Metadata["error"] = eventCtx.Error.Error()
	}

	return event
}

// buildOperationalEventType builds the operational event type from status
func buildOperationalEventType(status string) string {
	switch status {
	case OperationStatusStart:
		return "operation.start"
	case OperationStatusProgress:
		return "operation.progress"
	case OperationStatusComplete:
		return "operation.complete"
	case OperationStatusError:
		return "operation.error"
	case OperationStatusDependencyRef:
		return EventTypeLifecycleDependencyRef
	default:
		return "operation.unknown"
	}
}
