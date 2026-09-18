package mcp

import (
	"maps"
	"sync/atomic"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"

	"github.com/zqk-os/zqk/pkg/concurrency"
	"github.com/zqk-os/zqk/pkg/logging"
)

// ClientEventContext encapsulates the logic for recording client events
// It groups sequenceID and clientID retrieval, making event recording cleaner
// Similar to QueryContext, this eliminates scattered boolean checks and repeated code
type ClientEventContext struct {
	// Server reference for accessing sequenceID and clientID
	server *Server

	// Cached values (set by Compute())
	sequenceID          string
	clientID            string
	canRecord           bool
	computed            bool
	eventsRecordedTotal atomic.Int64
}

// GetClientEventContextStats returns lifetime counter for events recorded via this context.
func (c *ClientEventContext) GetClientEventContextStats() int64 {
	if c == nil {
		return 0
	}
	return c.eventsRecordedTotal.Load()
}

// NewClientEventContext creates a new ClientEventContext
func NewClientEventContext(server *Server) *ClientEventContext {
	return &ClientEventContext{
		server:   server,
		computed: false,
	}
}

// Compute retrieves sequenceID and clientID from the server
// This groups all the scattered mutex locks and checks into a single method
func (c *ClientEventContext) Compute() {
	if c.computed {
		return
	}

	if c.server == nil {
		c.canRecord = false
		c.computed = true
		return
	}

	// Get sequence ID
	_ = concurrency.RunInRLockWithLogger(
		&c.server.sequenceIDMu, LockNameClientEventContextGetSequenceId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			c.sequenceID = c.server.currentSequenceID
			return nil
		},
	)

	// Get client ID
	_ = concurrency.RunInRLockWithLogger(
		&c.server.clientIDMu, LockNameClientEventContextGetClientId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
		func() error {
			c.clientID = c.server.clientID
			return nil
		},
	)

	// Can record if both are set
	c.canRecord = c.sequenceID != emptyValue && c.clientID != emptyValue

	c.computed = true
}

// CanRecord returns whether we can record an event (both sequenceID and clientID are set)
func (c *ClientEventContext) CanRecord() bool {
	c.Compute()
	return c.canRecord
}

// GetSequenceID returns the current sequence ID
func (c *ClientEventContext) GetSequenceID() string {
	c.Compute()
	return c.sequenceID
}

// GetClientID returns the current client ID
func (c *ClientEventContext) GetClientID() string {
	c.Compute()
	return c.clientID
}

// RecordEvent records a client event if sequenceID is available
// clientID is optional - if not set, will use empty string or fallback to connecting ID
// This allows recording events even before clientID is fully established
// Metrics about the recording operation are tracked in the event fields
func (c *ClientEventContext) RecordEvent(eventType string, fields map[string]any) {
	startTime := time.Now()
	c.Compute()
	if c.sequenceID == emptyValue {
		return // Can't record without sequenceID
	}
	// Use clientID if available, otherwise use empty string (metrics store will handle it)
	clientID := c.clientID
	if clientID == emptyValue {
		// Try to get clientID from server directly (might have been set since last compute)
		_ = concurrency.RunInRLockWithLogger(
			&c.server.clientIDMu, LockNameClientEventContextRefreshClientId, logging.GetLockLoggerFromProfile(string(pkgctx.ProfileSystem)),
			func() error {
				clientID = c.server.clientID
				return nil
			},
		)
		// Update cached value if we found one
		if clientID != emptyValue {
			c.clientID = clientID
			c.canRecord = true
		}
	}

	// Add metrics about the recording operation to fields
	if fields == nil {
		fields = make(map[string]any)
	}
	// Track timing of event recording preparation
	fields["_record_prep_duration_ns"] = time.Since(startTime).Nanoseconds()
	c.eventsRecordedTotal.Add(1)
	c.server.recordClientEvent(c.sequenceID, clientID, eventType, fields)
}

// RecordEventWithClientID records a client event with an explicit clientID
// Useful for cases where clientID might not be set yet (e.g., connect, notification_initialized)
func (c *ClientEventContext) RecordEventWithClientID(eventType, clientID string, fields map[string]any) {
	c.Compute()
	if c.sequenceID == emptyValue {
		return
	}
	// Use provided clientID if available, otherwise use computed one
	if clientID == emptyValue {
		clientID = c.clientID
	}
	if clientID == emptyValue {
		return
	}
	c.eventsRecordedTotal.Add(1)
	c.server.recordClientEvent(c.sequenceID, clientID, eventType, fields)
}

// RecordDeprecatedFeature records usage of a deprecated feature for tracking
// component specifies the area/component where the deprecated feature exists (e.g., "tool_prefix", "api_endpoint", "config_option")
// additionalFields can include context like "tool_name", "endpoint", "option_name", "description", etc.
func (c *ClientEventContext) RecordDeprecatedFeature(component string, additionalFields map[string]any) {
	fields := make(map[string]any, len(additionalFields)+1)
	maps.Copy(fields, additionalFields)
	fields[logMapKeyComponent] = component
	c.RecordEvent("deprecated_feature_used", fields)
}

// Reset clears the computed values, forcing a recompute on next access
// Useful when you know the server state has changed
func (c *ClientEventContext) Reset() {
	c.computed = false
	c.sequenceID = emptyValue
	c.clientID = emptyValue
	c.canRecord = false
}
