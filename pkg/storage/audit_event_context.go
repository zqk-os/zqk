package storage

import (
	"sync/atomic"
)

var (
	// auditEventCreationDepth tracks the depth of audit event creation calls
	// Used to prevent cycles (creating audit events during audit event creation)
	auditEventCreationDepth int32
)

// IsCreatingAuditEvent returns true if we're currently in the process of creating an audit event
// Thread-safe (uses atomic operations)
func IsCreatingAuditEvent() bool {
	return atomic.LoadInt32(&auditEventCreationDepth) > 0
}

// BeginAuditEventCreation marks the start of audit event creation
// Returns a function to call when creation is complete (for defer)
// Thread-safe (uses atomic operations)
func BeginAuditEventCreation() func() {
	atomic.AddInt32(&auditEventCreationDepth, 1)
	return func() {
		atomic.AddInt32(&auditEventCreationDepth, -1)
	}
}
