package audit

import "github.com/zqk-os/zqk/pkg/objects"

const (
	Kind     = objects.KindAuditEvent
	SpecFile = "audit_event.yaml"

	StatusCompleted = "completed"

	FieldID             = "id"
	FieldTargetID       = "target_id"
	FieldEventType      = "event_type"
	FieldCreatedAt      = "created_at"
	FieldOccurrenceCnt  = "occurrence_count"
	FieldOccurrenceTs   = "occurrence_timestamps"
	FieldOriginSystem   = "origin_system"
	FieldOriginProject  = "origin_project"
	FieldNamespaceID    = "namespace_id"
	FieldOperation      = "operation"
	FieldSeverity       = "severity"
	FieldTitle          = "title"
	FieldCreatedBy      = "created_by"
	FieldUpdatedAt      = "updated_at"
	FieldUpdatedBy      = "updated_by"
	FieldTargetKind     = "target_kind"
	FieldTargetPath     = "target_path"
	FieldMetadata       = "metadata"
	FieldOriginalValue  = "original_value"
	FieldNewValue       = "new_value"
	FieldRecoveryMethod = "recovery_method"
	FieldReason         = "reason"
	FieldSessionID      = objects.FieldKeySessionID
)
