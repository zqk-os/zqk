package scheduler

const emptyValue = ""

// AuditEventFields holds extracted fields from an audit event
// Provides a clean, efficient way to access common audit event fields
type AuditEventFields struct {
	TargetID   string
	TargetKind string
	EventType  string
	CreatedAt  string
	Metadata   map[string]any
}

// ExtractAuditEventFields extracts common fields from an audit event
// Uses dynamically loaded field names from the audit_event spec
func ExtractAuditEventFields(event map[string]any) *AuditEventFields {
	return &AuditEventFields{
		TargetID:   getString(event, getAuditEventTargetIDField()),
		TargetKind: getString(event, getAuditEventTargetKindField()),
		EventType:  getString(event, getAuditEventEventTypeField()),
		CreatedAt:  getString(event, getAuditEventCreatedAtField()),
		Metadata:   getMetadata(event),
	}
}

// IsSchedulerJobEvent checks if this event is for a scheduler job
// Uses cached scheduler job kind for efficient comparison
func (a *AuditEventFields) IsSchedulerJobEvent() bool {
	return a.TargetKind == getSchedulerJobKind()
}

// IsSchedulerJobEventType checks if this event has a scheduler job event type
// Uses cached event type lookup for efficient comparison
func (a *AuditEventFields) IsSchedulerJobEventType() bool {
	return isSchedulerJobEventType(a.EventType)
}

// MatchesJobIDFilter checks if this event matches the given job ID filter
// Returns true if filter is empty (no filter) or if target_id matches filter
func (a *AuditEventFields) MatchesJobIDFilter(jobIDFilter string) bool {
	if jobIDFilter == emptyValue {
		return true
	}
	return a.TargetID == jobIDFilter
}

// IsCompletionEvent checks if this event is a completion event (completed or failed)
func (a *AuditEventFields) IsCompletionEvent() bool {
	return isSchedulerJobCompletionEventType(a.EventType)
}

// IsStartedEvent checks if this event is a started event
func (a *AuditEventFields) IsStartedEvent() bool {
	return isSchedulerJobStartedEventType(a.EventType)
}

// IsCompletedEvent checks if this event is a completed event
func (a *AuditEventFields) IsCompletedEvent() bool {
	return isSchedulerJobCompletedEventType(a.EventType)
}

// IsFailedEvent checks if this event is a failed event
func (a *AuditEventFields) IsFailedEvent() bool {
	return isSchedulerJobFailedEventType(a.EventType)
}

// ShouldProcess checks if this event should be processed for scheduler job history/activity
// Combines multiple checks into a single efficient call
func (a *AuditEventFields) ShouldProcess(jobIDFilter string) bool {
	return a.IsSchedulerJobEvent() && a.IsSchedulerJobEventType() && a.MatchesJobIDFilter(jobIDFilter)
}

// getMetadata safely extracts metadata from an audit event
func getMetadata(event map[string]any) map[string]any {
	if metadata, ok := event[getAuditEventMetadataField()].(map[string]any); ok {
		return metadata
	}
	return nil
}
