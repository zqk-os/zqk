package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lanceman/zqk/pkg/concurrency"
	pkgctx "github.com/lanceman/zqk/pkg/context"
	"github.com/lanceman/zqk/pkg/errfmt"
	"github.com/lanceman/zqk/pkg/logging"
)

// NotificationSentinel is a special error type that indicates a notification
// was processed and no response should be sent
type NotificationSentinel struct{}

func (n *NotificationSentinel) Error() string {
	return "notification processed (no response)"
}

// EventsSubscribeParams represents parameters for events/subscribe
type EventsSubscribeParams struct {
	EventTypes []string `json:"eventTypes"`         // Event types to subscribe to (empty = all)
	ClientID   string   `json:"clientId,omitempty"` // Optional client identifier
}

// EventsSubscribeResult represents the result of subscribing to events
type EventsSubscribeResult struct {
	SubscriptionID string   `json:"subscriptionId"`
	EventTypes     []string `json:"eventTypes"`
}

// handleEventsSubscribe handles the events/subscribe method
func (s *Server) handleEventsSubscribe(_ context.Context, _ string, params json.RawMessage) (any, error) {
	var subscribeParams EventsSubscribeParams
	if err := json.Unmarshal(params, &subscribeParams); err != nil {
		return nil, &JSONRPCError{
			Code:    InvalidParams,
			Message: "Invalid parameters",
		}
	}

	// Record event using context object
	eventCtx := s.getClientEventContext()
	eventTypeStrings := append(make([]string, 0, len(subscribeParams.EventTypes)), subscribeParams.EventTypes...)
	eventCtx.RecordEvent("events_subscribe", map[string]any{
		"event_types": eventTypeStrings,
	})

	// Convert string event types to EventType
	eventTypes := make([]EventType, 0, len(subscribeParams.EventTypes))
	for _, et := range subscribeParams.EventTypes {
		eventTypes = append(eventTypes, EventType(et))
	}

	// If no event types specified, subscribe to all
	if len(eventTypes) == 0 {
		eventTypes = []EventType{
			EventTypeLogDebug, EventTypeLogInfo, EventTypeLogWarn, EventTypeLogError,
			EventTypePermissionDenied, EventTypePermissionGranted,
			EventTypeToolStarted, EventTypeToolCompleted, EventTypeToolFailed,
			EventTypeSystemWarning, EventTypeSystemError,
			EventTypePromptAvailable, EventTypeActionRequired,
		}
	}

	// Generate subscription ID
	// Use client ID from server if not provided in params
	var clientID string
	if subscribeParams.ClientID != emptyValue {
		clientID = subscribeParams.ClientID
	} else {
		_ = concurrency.RunInRLockWithLogger(
			&s.clientIDMu, LockNameMcpServerEventsClientId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				clientID = s.clientID
				return nil
			},
		)
	}

	subscriptionID := fmt.Sprintf("sub_%d", time.Now().UnixNano())
	if clientID != emptyValue {
		subscriptionID = fmt.Sprintf("%s_%s", clientID, subscriptionID)
	}

	// Create write function that uses the server's transport
	writeFunc := func(data []byte) error {
		var writer *bufio.Writer
		var format *MessageFormat
		_ = concurrency.RunInRLockWithLogger(
			&s.transportMu, LockNameMcpServerEventsTransport, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				writer = s.transportWriter
				format = s.transportFormat
				return nil
			},
		)

		if writer == nil || format == nil {
			return errfmt.Errorf("transport not available")
		}

		// Use the transport to write the notification
		transport := NewDefaultTransport()
		return transport.WriteMessage(writer, data, format)
	}

	// Create subscriber
	subscriber := NewMCPEventSubscriber(
		subscriptionID,
		eventTypes,
		writeFunc,
		5*time.Minute, // 5 minute idle timeout
	)

	// Subscribe to event emitter
	if s.eventEmitter != nil {
		s.eventEmitter.Subscribe(subscriber)
	}

	return EventsSubscribeResult{
		SubscriptionID: subscriptionID,
		EventTypes:     subscribeParams.EventTypes,
	}, nil
}

// handleEventsUnsubscribe handles the events/unsubscribe method
func (s *Server) handleEventsUnsubscribe(_ context.Context, _ string, params json.RawMessage) (any, error) {
	var unsubscribeParams struct {
		SubscriptionID string `json:"subscriptionId"`
	}
	if err := json.Unmarshal(params, &unsubscribeParams); err != nil {
		return nil, &JSONRPCError{
			Code:    InvalidParams,
			Message: "Invalid parameters",
		}
	}

	// Record event using context object
	eventCtx := s.getClientEventContext()
	eventCtx.RecordEvent("events_unsubscribe", map[string]any{
		"subscription_id": unsubscribeParams.SubscriptionID,
	})

	if s.eventEmitter != nil {
		s.eventEmitter.Unsubscribe(unsubscribeParams.SubscriptionID)
	}

	return map[string]any{
		"message": "Unsubscribed successfully",
	}, nil
}

// handleEventsList handles the events/list method
func (s *Server) handleEventsList(_ context.Context, _ string, _ json.RawMessage) (any, error) {
	// Record event using context object
	eventCtx := s.getClientEventContext()
	eventCtx.RecordEvent("events_list", map[string]any{})

	// Return available event types
	eventTypes := []string{
		string(EventTypeLogDebug),
		string(EventTypeLogInfo),
		string(EventTypeLogWarn),
		string(EventTypeLogError),
		string(EventTypePermissionDenied),
		string(EventTypePermissionGranted),
		string(EventTypeUserDeactivated),
		string(EventTypeUserActivated),
		string(EventTypeToolStarted),
		string(EventTypeToolCompleted),
		string(EventTypeToolFailed),
		string(EventTypeSystemWarning),
		string(EventTypeSystemError),
		string(EventTypePromptAvailable),
		string(EventTypeActionRequired),
	}

	subscriberCount := 0
	if s.eventEmitter != nil {
		subscriberCount = s.eventEmitter.GetSubscriberCount()
	}

	return map[string]any{
		"eventTypes":      eventTypes,
		"subscriberCount": subscriberCount,
	}, nil
}
