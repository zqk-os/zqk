package storage

import "github.com/zqk-os/zqk/pkg/storage/audit"

// IsCreatingAuditEvent returns true if we're currently in the process of creating an audit event.
func IsCreatingAuditEvent() bool {
	return audit.IsCreatingEvent()
}

// BeginAuditEventCreation marks the start of audit event creation.
func BeginAuditEventCreation() func() {
	return audit.BeginEventCreation()
}
