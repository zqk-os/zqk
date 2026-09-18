package system

import (
	"fmt"

	"github.com/zqk-os/zqk/pkg/coordination"
	"github.com/zqk-os/zqk/pkg/logging"
)

// CacheEventSubscriber subscribes to cache-related operational events for debugging and monitoring
type CacheEventSubscriber struct {
	id      string
	active  bool
	logger  logging.Logger
	profile string
}

const cacheSubscriberID = "cache-event-subscriber"

// NewCacheEventSubscriber creates a new cache event subscriber
func NewCacheEventSubscriber(profile string) *CacheEventSubscriber {
	return &CacheEventSubscriber{
		id:      cacheSubscriberID,
		active:  true,
		profile: profile,
		logger:  logging.GetLoggerFromProfile(profile),
	}
}

// ID returns the subscriber ID
func (c *CacheEventSubscriber) ID() string {
	return c.id
}

// HandleEvent processes cache-related operational events
func (c *CacheEventSubscriber) HandleEvent(event *coordination.OperationalEvent) error {
	// Only handle cache-related operations
	if event.OperationType != eventTypeCacheOperation && event.OperationType != eventTypeCacheAvailability {
		return nil // Not a cache event, ignore
	}

	// Extract cache-specific metadata
	cacheOperation := ""
	entryCount := 0
	available := false
	operation := ""

	// Extract metadata from EventData (which is included in Metadata)
	if event.Metadata != nil {
		// Check for cache_operation in metadata (from LoggingFields)
		if op, ok := event.Metadata[eventKeyCacheOperation].(string); ok {
			cacheOperation = op
		}
		// Check for entry_count in metadata (from LoggingFields)
		if count, ok := event.Metadata[eventKeyEntryCount].(int); ok {
			entryCount = count
		} else if countFloat, ok := event.Metadata[eventKeyEntryCount].(float64); ok {
			// Handle JSON number conversion
			entryCount = int(countFloat)
		}
		// Check for cache_available in metadata (from LoggingFields)
		if avail, ok := event.Metadata[eventKeyCacheAvailable].(bool); ok {
			available = avail
		}
		// Check for operation in metadata
		if op, ok := event.Metadata[eventKeyOperation].(string); ok {
			operation = op
		}
	}

	// Log cache events for debugging
	switch event.OperationType {
	case eventTypeCacheOperation:
		switch cacheOperation {
		case "build", "load", "rebuild":
			logging.Fluent(c.logger).Debug("Cache build event via coordinator").
				String("operation", cacheOperation).
				String("status", event.Status).
				EntryCount(entryCount).
				String("operation_id", event.OperationID).
				Log()
		case "save":
			logging.Fluent(c.logger).Debug("Cache save event via coordinator").
				String("status", event.Status).
				EntryCount(entryCount).
				String("operation_id", event.OperationID).
				Log()
		}
	case eventTypeCacheAvailability:
		// Log cache availability events, especially errors
		if event.Status == eventStatusError || !available {
			logging.Fluent(c.logger).Warn("Cache availability event via coordinator").
				String("operation", operation).
				String("status", event.Status).
				String("available", fmt.Sprintf("%v", available)).
				EntryCount(entryCount).
				String("operation_id", event.OperationID).
				Log()
		} else {
			logging.Fluent(c.logger).Debug("Cache availability event via coordinator").
				String("operation", operation).
				String("status", event.Status).
				String("available", fmt.Sprintf("%v", available)).
				EntryCount(entryCount).
				String("operation_id", event.OperationID).
				Log()
		}
	}

	return nil
}

// EventTypes returns the event types this subscriber is interested in
// Empty slice means subscribe to all operational events
// We filter by OperationType in HandleEvent instead
func (c *CacheEventSubscriber) EventTypes() []string {
	// Subscribe to all operational events, filter by OperationType in HandleEvent
	return []string{}
}

// IsActive returns whether the subscriber is still active
func (c *CacheEventSubscriber) IsActive() bool {
	return c.active
}

// Deactivate deactivates the subscriber
func (c *CacheEventSubscriber) Deactivate() {
	c.active = false
}
