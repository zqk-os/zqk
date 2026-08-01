package mcp

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/goroutinelabels"
	"github.com/lanceman/zqk/pkg/logging"
)

// EventType represents the type of event being emitted
type EventType string

const (
	// Log events
	EventTypeLogDebug EventType = "log.debug"
	EventTypeLogInfo  EventType = "log.info"
	EventTypeLogWarn  EventType = "log.warn"
	EventTypeLogError EventType = "log.error"

	// Permission events
	EventTypePermissionDenied  EventType = "permission.denied"
	EventTypePermissionGranted EventType = "permission.granted"
	EventTypeUserDeactivated   EventType = "user.deactivated"
	EventTypeUserActivated     EventType = "user.activated"

	// Tool execution events
	EventTypeToolStarted   EventType = "tool.started"
	EventTypeToolCompleted EventType = "tool.completed"
	EventTypeToolFailed    EventType = "tool.failed"

	// System events
	EventTypeSystemWarning EventType = "system.warning"
	EventTypeSystemError   EventType = "system.error"

	// Prompt events (for future use)
	EventTypePromptAvailable EventType = "prompt.available"
	EventTypeActionRequired  EventType = "action.required"
)

// Event represents a structured event that can be emitted and subscribed to
type Event struct {
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	Severity  string         `json:"severity,omitempty"` // "debug", "info", "warn", "error"
	Priority  string         `json:"priority,omitempty"` // "low", "medium", "high", "critical"
}

// EventSubscriber represents a subscriber to events
// This allows MCP clients to receive real-time notifications
type EventSubscriber interface {
	// ID returns a unique identifier for this subscriber
	ID() string

	// SendEvent sends an event to the subscriber
	// Returns false if the subscriber is no longer active
	SendEvent(event *Event) bool

	// EventTypes returns the event types this subscriber is interested in
	EventTypes() []EventType

	// IsActive returns whether the subscriber is still active
	IsActive() bool
}

// EventEmitter manages event subscriptions and emission
// This provides a pub/sub pattern for MCP notifications
type EventEmitter struct {
	mu                 sync.RWMutex
	subscribers        map[string]EventSubscriber // subscriber ID -> subscriber
	typeIndex          map[EventType][]string     // event type -> subscriber IDs
	bufferSize         int                        // Buffer size for event channels
	eventsEmittedTotal atomic.Int64
	subscriptionsTotal atomic.Int64
}

// GetEventEmitterStats returns lifetime counters for total events emitted and subscriptions created.
func (ee *EventEmitter) GetEventEmitterStats() (emitted, subscriptions int64) {
	if ee == nil {
		return 0, 0
	}
	return ee.eventsEmittedTotal.Load(), ee.subscriptionsTotal.Load()
}

// NewEventEmitter creates a new event emitter
func NewEventEmitter(bufferSize int) *EventEmitter {
	if bufferSize <= 0 {
		bufferSize = DefaultEventBufferSize
	}
	return &EventEmitter{
		subscribers: make(map[string]EventSubscriber),
		typeIndex:   make(map[EventType][]string),
		bufferSize:  bufferSize,
	}
}

// GetBufferSize returns the buffer size for event channels
func (ee *EventEmitter) GetBufferSize() int {
	var bufferSize int
	_ = concurrency.RunInRLockWithLogger(
		&ee.mu, LockNameEventEmitterGetBufferSize, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			bufferSize = ee.bufferSize
			return nil
		},
	)
	return bufferSize
}

// Subscribe adds a subscriber for specific event types
// Returns the subscriber ID
func (ee *EventEmitter) Subscribe(subscriber EventSubscriber) string {
	ee.subscriptionsTotal.Add(1)
	var subscriberID string
	var eventTypes []EventType
	_ = concurrency.RunInLockWithLogger(
		&ee.mu, LockNameEventEmitterSubscribe, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			subscriberID = subscriber.ID()
			ee.subscribers[subscriberID] = subscriber
			eventTypes = subscriber.EventTypes()
			return nil
		},
	)

	// Index by event type for fast lookup (re-acquire lock)
	_ = concurrency.RunInLockWithLogger(
		&ee.mu, LockNameEventEmitterSubscribeIndex, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, eventType := range eventTypes {
				ee.typeIndex[eventType] = append(ee.typeIndex[eventType], subscriberID)
			}
			return nil
		},
	)

	return subscriberID
}

// Unsubscribe removes a subscriber
func (ee *EventEmitter) Unsubscribe(subscriberID string) {
	var subscriber EventSubscriber
	var exists bool
	var eventTypes []EventType
	_ = concurrency.RunInLockWithLogger(
		&ee.mu, LockNameEventEmitterUnsubscribeCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			var ok bool
			subscriber, ok = ee.subscribers[subscriberID]
			exists = ok
			if exists {
				eventTypes = subscriber.EventTypes()
			}
			return nil
		},
	)

	if !exists {
		return
	}

	// Remove from type index (re-acquire lock)
	_ = concurrency.RunInLockWithLogger(
		&ee.mu, LockNameEventEmitterUnsubscribeRemove, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			// Remove from type index
			for _, eventType := range eventTypes {
				ids := ee.typeIndex[eventType]
				for i, id := range ids {
					if id == subscriberID {
						ee.typeIndex[eventType] = append(ids[:i], ids[i+1:]...)
						break
					}
				}
			}
			delete(ee.subscribers, subscriberID)
			return nil
		},
	)
}

// Emit sends an event to all subscribers interested in that event type
// Returns the number of subscribers that received the event
func (ee *EventEmitter) Emit(event *Event) int {
	if event == nil {
		return 0
	}

	ee.eventsEmittedTotal.Add(1)

	// Set timestamp if not set
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	var subscribers []EventSubscriber
	_ = concurrency.RunInRLockWithLogger(
		&ee.mu, LockNameEventEmitterEmitCopy, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			subscriberIDs := ee.typeIndex[event.Type]
			subscribers = make([]EventSubscriber, 0, len(subscriberIDs))
			for _, id := range subscriberIDs {
				if sub, exists := ee.subscribers[id]; exists {
					subscribers = append(subscribers, sub)
				}
			}
			return nil
		},
	)

	// Send to all subscribers (non-blocking)
	sentCount := 0
	for _, subscriber := range subscribers {
		if !subscriber.IsActive() {
			// Clean up inactive subscribers
			subID := subscriber.ID()
			goroutinelabels.NewGoroutine("mcp_event_emitter_unsubscribe", fmt.Sprintf("unsubscribing inactive subscriber %s", subID)).
				StartSimple(func() {
					ee.Unsubscribe(subID)
				})
			continue
		}

		if subscriber.SendEvent(event) {
			sentCount++
		} else {
			// Subscriber rejected event - mark as inactive
			subID := subscriber.ID()
			goroutinelabels.NewGoroutine("mcp_event_emitter_unsubscribe", fmt.Sprintf("unsubscribing rejected subscriber %s", subID)).
				StartSimple(func() {
					ee.Unsubscribe(subID)
				})
		}
	}

	return sentCount
}

// GetSubscriberCount returns the number of active subscribers
func (ee *EventEmitter) GetSubscriberCount() int {
	var count int
	_ = concurrency.RunInRLockWithLogger(
		&ee.mu, LockNameEventEmitterGetSubscriberCount, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			count = len(ee.subscribers)
			return nil
		},
	)
	return count
}

// GetSubscriberCountByType returns the number of subscribers for a specific event type
func (ee *EventEmitter) GetSubscriberCountByType(eventType EventType) int {
	var count int
	_ = concurrency.RunInRLockWithLogger(
		&ee.mu, LockNameEventEmitterGetSubscriberCountByType, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			count = len(ee.typeIndex[eventType])
			return nil
		},
	)
	return count
}

// Cleanup removes all inactive subscribers
func (ee *EventEmitter) Cleanup() {
	var inactiveIDs []string
	var subscribersToClean map[string]EventSubscriber
	_ = concurrency.RunInRLockWithLogger(
		&ee.mu, LockNameEventEmitterCleanupCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			inactiveIDs = make([]string, 0)
			subscribersToClean = make(map[string]EventSubscriber)
			for id, subscriber := range ee.subscribers {
				if !subscriber.IsActive() {
					inactiveIDs = append(inactiveIDs, id)
					subscribersToClean[id] = subscriber
				}
			}
			return nil
		},
	)

	if len(inactiveIDs) == 0 {
		return
	}

	// Remove inactive subscribers (re-acquire lock)
	_ = concurrency.RunInLockWithLogger(
		&ee.mu, LockNameEventEmitterCleanupRemove, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			for _, id := range inactiveIDs {
				// Remove from type index
				subscriber := subscribersToClean[id]
				for _, eventType := range subscriber.EventTypes() {
					ids := ee.typeIndex[eventType]
					for i, sid := range ids {
						if sid == id {
							ee.typeIndex[eventType] = append(ids[:i], ids[i+1:]...)
							break
						}
					}
				}
				delete(ee.subscribers, id)
			}
			return nil
		},
	)
}
