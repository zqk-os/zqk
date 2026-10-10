package storage

import (
	"context"

	"github.com/zqk-os/zqk/pkg/storage/audit"
)

// IsCreatingAuditEventContext returns true if the context or goroutine is actively creating an audit event.
func IsCreatingAuditEventContext(ctx context.Context) bool {
	return audit.IsCreatingEventContext(ctx)
}

// IsCreatingAuditEvent returns true if we're currently in the process of creating an audit event.
func IsCreatingAuditEvent() bool {
	return audit.IsCreatingEvent()
}

// BeginAuditEventCreation marks the start of audit event creation.
func BeginAuditEventCreation() func() {
	return audit.BeginEventCreation()
}
