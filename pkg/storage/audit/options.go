package audit

import (
	"strings"

	"github.com/lanceman/zqk/pkg/zqkenv"
)

// EventOptions is the audit subpackage contract for creating an audit event.
// Package storage aliases this as AuditEventOptions so existing callers stay stable.
type EventOptions struct {
	EventType            string         // Required: event type (object_creation, object_update, object_deletion, etc.)
	Operation            string         // Required: operation description
	Severity             string         // Required: severity (low, medium, high)
	TargetKind           string         // Optional: target object kind
	TargetID             string         // Optional: target object ID
	TargetPath           string         // Optional: target file path
	Metadata             map[string]any // Optional: additional metadata
	OriginalValue        string         // Optional: original value for change events
	NewValue             string         // Optional: new value for change events
	RecoveryMethod       string         // Optional: recovery method
	Reason               any            // Optional: reason
	SessionID            string         // Optional: author session ID
	CreatedAt            string         // Optional: custom created_at timestamp (defaults to now)
	CreatedBy            string         // Optional: custom created_by (defaults to secCtx.AccountID or "system")
	UpdatedAt            string         // Optional: custom updated_at timestamp (defaults to CreatedAt)
	OccurrenceCount      int            // Optional: occurrence count for aggregated events
	OccurrenceTimestamps []string       // Optional: occurrence timestamps for aggregated events
	OnError              func(error)    // Optional: callback to receive creation errors (for test verification)
	OnBuffered           func()         // Optional: callback to indicate event was buffered (for test verification)
}

// ApplySessionEnv fills SessionID from the process environment when unset.
// TRACK: session id on audit events — env fallback when options unset.
func ApplySessionEnv(options *EventOptions) {
	if options == nil || options.SessionID != "" {
		return
	}
	if v := strings.TrimSpace(zqkenv.SessionID().Get()); v != "" {
		options.SessionID = v
	}
}
