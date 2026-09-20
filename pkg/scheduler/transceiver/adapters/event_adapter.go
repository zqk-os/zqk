package adapters

import (
	"context"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/errfmt"
	"github.com/zqk-os/zqk/pkg/logging"
	"github.com/zqk-os/zqk/pkg/scheduler/transceiver/types"
)

// EventAdapter implements the ProtocolAdapter interface for internal event bus
// Currently a placeholder that logs events - will integrate with event bus when implemented
type EventAdapter struct {
	logger logging.Logger
	// Future: eventBus *EventBus
}

// NewEventAdapter creates a new event adapter
func NewEventAdapter(logger logging.Logger) *EventAdapter {
	if logger == nil {
		logger = logging.GetLoggerFromProfile(string(pkgctx.ProfileSystem))
	}
	return &EventAdapter{logger: logger}
}

// Name returns the protocol name
func (a *EventAdapter) Name() string {
	return "event"
}

// Validate validates action configuration for event protocol
func (a *EventAdapter) Validate(action types.Action) error {
	if action.Endpoint == emptyValue {
		return errfmt.Errorf("endpoint (event name) required for event protocol")
	}
	return nil
}

// Send sends a message by emitting an internal event
// Currently logs the event - will integrate with event bus when implemented
//
//nolint:gocritic // Message passed by value to avoid mutation during routing
func (a *EventAdapter) Send(ctx context.Context, message types.Message, action types.Action) error {
	// TODO: Integrate with event bus when implemented
	// For now, just log the event
	logging.Fluent(a.logger).Info(LogEventSchedulerTransceiverEventEmittedStub).
		String("event_name", action.Endpoint).
		EventType(message.EventType).
		String("source", message.Source).
		Log()

	// Future implementation:
	// return a.eventBus.Emit(ctx, action.Endpoint, message.Payload)

	return nil
}
