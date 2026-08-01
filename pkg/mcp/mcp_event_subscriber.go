package mcp

import (
	"encoding/json"
	"sync"
	"sync/atomic"
	"time"

	pkgctx "github.com/lanceman/zqk/pkg/context"

	"github.com/lanceman/zqk/pkg/concurrency"
	"github.com/lanceman/zqk/pkg/logging"
)

// MCPEventSubscriber is an EventSubscriber that sends events via MCP notifications
// This allows MCP clients to receive real-time events as JSON-RPC notifications
type MCPEventSubscriber struct {
	id              string
	eventTypes      []EventType
	writeFunc       func([]byte) error // Function to write JSON-RPC notification
	active          atomic.Bool
	eventsSentTotal atomic.Int64
	mu              sync.RWMutex
	lastActivity    time.Time
	maxIdle         time.Duration
}

// GetMCPEventSubscriberStats returns lifetime counter for events sent.
func (s *MCPEventSubscriber) GetMCPEventSubscriberStats() int64 {
	if s == nil {
		return 0
	}
	return s.eventsSentTotal.Load()
}

// NewMCPEventSubscriber creates a new MCP event subscriber
// writeFunc should write a JSON-RPC notification to the client
// maxIdle is the maximum time without activity before the subscriber is considered inactive
func NewMCPEventSubscriber(
	id string,
	eventTypes []EventType,
	writeFunc func([]byte) error,
	maxIdle time.Duration,
) *MCPEventSubscriber {
	if maxIdle <= 0 {
		maxIdle = 5 * time.Minute // Default: 5 minutes
	}
	sub := &MCPEventSubscriber{
		id:           id,
		eventTypes:   eventTypes,
		writeFunc:    writeFunc,
		lastActivity: time.Now(),
		maxIdle:      maxIdle,
	}
	sub.active.Store(true)
	return sub
}

// ID returns the subscriber ID
func (s *MCPEventSubscriber) ID() string {
	return s.id
}

// EventTypes returns the event types this subscriber is interested in
func (s *MCPEventSubscriber) EventTypes() []EventType {
	var eventTypes []EventType
	_ = concurrency.RunInRLockWithLogger(
		&s.mu, LockNameMcpEventSubscriberGetTypes, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			eventTypes = s.eventTypes
			return nil
		},
	)
	return eventTypes
}

// SendEvent sends an event to the subscriber via MCP notification
// Returns false if the subscriber is no longer active or write failed
func (s *MCPEventSubscriber) SendEvent(event *Event) bool {
	if !s.active.Load() {
		return false
	}

	var maxIdle time.Duration
	var writeFunc func([]byte) error
	var lastActivity time.Time
	_ = concurrency.RunInLockWithLogger(
		&s.mu, LockNameMcpEventSubscriberSendCheck, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			maxIdle = s.maxIdle
			writeFunc = s.writeFunc
			lastActivity = s.lastActivity
			return nil
		},
	)

	// Check if subscriber is idle (outside lock)
	if time.Since(lastActivity) > maxIdle {
		s.active.Store(false)
		return false
	}

	// Create JSON-RPC notification (outside lock)
	notification := map[string]any{
		jsonrpcFieldJSONRPC: JSONRPCVersion,
		jsonrpcFieldMethod:  notificationMethodEvent,
		jsonrpcFieldParams: map[string]any{
			"event": event,
		},
	}

	// Marshal to JSON (outside lock)
	data, err := json.Marshal(notification)
	if err != nil {
		// Failed to marshal - mark as inactive
		s.active.Store(false)
		return false
	}

	// Write to client (outside lock - I/O operation)
	if err := writeFunc(data); err != nil {
		// Write failed - mark as inactive
		s.active.Store(false)
		return false
	}

	s.eventsSentTotal.Add(1)

	// Update last activity (re-acquire lock)
	_ = concurrency.RunInLockWithLogger(
		&s.mu, LockNameMcpEventSubscriberSendUpdate, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.lastActivity = time.Now()
			return nil
		},
	)
	return true
}

// IsActive returns whether the subscriber is still active
func (s *MCPEventSubscriber) IsActive() bool {
	var lastActivity time.Time
	var maxIdle time.Duration
	_ = concurrency.RunInRLockWithLogger(
		&s.mu, LockNameMcpEventSubscriberIsActive, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			lastActivity = s.lastActivity
			maxIdle = s.maxIdle
			return nil
		},
	)
	return s.active.Load() && time.Since(lastActivity) <= maxIdle
}

// Deactivate marks the subscriber as inactive
func (s *MCPEventSubscriber) Deactivate() {
	s.active.Store(false)
}

// UpdateEventTypes updates the event types this subscriber is interested in
func (s *MCPEventSubscriber) UpdateEventTypes(eventTypes []EventType) {
	_ = concurrency.RunInLockWithLogger(
		&s.mu, LockNameMcpEventSubscriberUpdateTypes, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			s.eventTypes = eventTypes
			s.lastActivity = time.Now()
			return nil
		},
	)
}
