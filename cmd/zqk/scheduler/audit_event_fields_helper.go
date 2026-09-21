package scheduler

import (
	"github.com/zqk-os/zqk/pkg/objects"
)

const (
	auditEventFieldTargetID   = "target_id"
	auditEventFieldTargetKind = "target_kind"
	auditEventFieldEventType  = "event_type"
	auditEventFieldCreatedAt  = "created_at"
	auditEventFieldMetadata   = "metadata"
)

func auditEventFieldNames() map[string]string {
	fallback := map[string]string{
		auditEventFieldTargetID:   auditEventFieldTargetID,
		auditEventFieldTargetKind: auditEventFieldTargetKind,
		auditEventFieldEventType:  auditEventFieldEventType,
		auditEventFieldCreatedAt:  auditEventFieldCreatedAt,
		auditEventFieldMetadata:   auditEventFieldMetadata,
	}
	spec, err := objects.GetGlobalSpecLoader().LoadSpecWithInheritance("audit_event.yaml")
	if err != nil || spec == nil || spec.ResolvedFields == nil {
		return fallback
	}
	fields := make(map[string]string, len(spec.ResolvedFields))
	for fieldName := range spec.ResolvedFields {
		fields[fieldName] = fieldName
	}
	if len(fields) == 0 {
		return fallback
	}
	return fields
}

func getAuditEventFieldName(fieldName string) string {
	if cached, ok := auditEventFieldNames()[fieldName]; ok {
		return cached
	}
	return fieldName
}

func getAuditEventTargetIDField() string {
	return getAuditEventFieldName(auditEventFieldTargetID)
}

func getAuditEventTargetKindField() string {
	return getAuditEventFieldName(auditEventFieldTargetKind)
}

func getAuditEventEventTypeField() string {
	return getAuditEventFieldName(auditEventFieldEventType)
}

func getAuditEventCreatedAtField() string {
	return getAuditEventFieldName(auditEventFieldCreatedAt)
}

func getAuditEventMetadataField() string {
	return getAuditEventFieldName(auditEventFieldMetadata)
}

func GetMetadataSuccessField() string {
	return "success"
}

func GetMetadataDurationField() string {
	return "duration_seconds"
}

func GetMetadataErrorField() string {
	return "error"
}
