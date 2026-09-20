package scheduler

import (
	"sync"

	"github.com/zqk-os/zqk/pkg/objects"
)

var (
	// Cached audit event field names (loaded from audit_event spec)
	auditEventFieldNamesCache     map[string]string
	auditEventFieldNamesCacheOnce sync.Once
	auditEventFieldNamesCacheErr  error
)

const (
	auditEventFieldTargetID   = "target_id"
	auditEventFieldTargetKind = "target_kind"
	auditEventFieldEventType  = "event_type"
	auditEventFieldCreatedAt  = "created_at"
	auditEventFieldMetadata   = "metadata"
)

// getAuditEventFieldName returns the field name for a given audit event field
// Dynamically loaded from the audit_event spec and cached for performance
// Falls back to the provided field name if spec can't be loaded
func getAuditEventFieldName(fieldName string) string {
	auditEventFieldNamesCacheOnce.Do(func() {
		// Load audit_event spec
		specLoader := objects.GetGlobalSpecLoader()
		spec, err := specLoader.LoadSpecWithInheritance("audit_event.yaml")
		if err != nil {
			auditEventFieldNamesCacheErr = err
			// Fallback to known values if spec can't be loaded
			auditEventFieldNamesCache = map[string]string{
				auditEventFieldTargetID:   auditEventFieldTargetID,
				auditEventFieldTargetKind: auditEventFieldTargetKind,
				auditEventFieldEventType:  auditEventFieldEventType,
				auditEventFieldCreatedAt:  auditEventFieldCreatedAt,
				auditEventFieldMetadata:   auditEventFieldMetadata,
			}
			return
		}

		// Extract field names from spec
		fields := make(map[string]string)
		if spec != nil && spec.ResolvedFields != nil {
			for fieldName := range spec.ResolvedFields {
				// Use the field name as-is (field names in specs match JSON field names)
				fields[fieldName] = fieldName
			}
		}

		// If no fields found, fallback to known values
		if len(fields) == 0 {
			fields = map[string]string{
				auditEventFieldTargetID:   auditEventFieldTargetID,
				auditEventFieldTargetKind: auditEventFieldTargetKind,
				auditEventFieldEventType:  auditEventFieldEventType,
				auditEventFieldCreatedAt:  auditEventFieldCreatedAt,
				auditEventFieldMetadata:   auditEventFieldMetadata,
			}
		}

		auditEventFieldNamesCache = fields
	})

	// Return cached field name, or fallback to provided field name
	if cached, ok := auditEventFieldNamesCache[fieldName]; ok {
		return cached
	}
	return fieldName
}

// getAuditEventTargetIDField returns the target_id field name
func getAuditEventTargetIDField() string {
	return getAuditEventFieldName(auditEventFieldTargetID)
}

// getAuditEventTargetKindField returns the target_kind field name
func getAuditEventTargetKindField() string {
	return getAuditEventFieldName(auditEventFieldTargetKind)
}

// getAuditEventEventTypeField returns the event_type field name
func getAuditEventEventTypeField() string {
	return getAuditEventFieldName(auditEventFieldEventType)
}

// getAuditEventCreatedAtField returns the created_at field name
func getAuditEventCreatedAtField() string {
	return getAuditEventFieldName(auditEventFieldCreatedAt)
}

// getAuditEventMetadataField returns the metadata field name
func getAuditEventMetadataField() string {
	return getAuditEventFieldName(auditEventFieldMetadata)
}

// GetMetadataSuccessField returns the field name for success in metadata
// This is a nested field within metadata, not a top-level audit event field
func GetMetadataSuccessField() string {
	return "success"
}

// GetMetadataDurationField returns the field name for duration_seconds in metadata
func GetMetadataDurationField() string {
	return "duration_seconds"
}

// GetMetadataErrorField returns the field name for error in metadata
func GetMetadataErrorField() string {
	return "error"
}
