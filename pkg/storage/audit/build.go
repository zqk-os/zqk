package audit

import (
	"path/filepath"
	"time"

	pkgctx "github.com/zqk-os/zqk/pkg/context"
	"github.com/zqk-os/zqk/pkg/specbuilder/bldr_enum_v1/audit_event"
	"github.com/zqk-os/zqk/pkg/specbuilder/instance_builders"
	"github.com/zqk-os/zqk/pkg/validation"
)

// ResolveActor picks created_by from options, then the security context, then system.
func ResolveActor(secCtx *pkgctx.SecurityContext, createdBy string) string {
	if createdBy != "" {
		return createdBy
	}
	if secCtx != nil && secCtx.AccountID != "" {
		return secCtx.AccountID
	}
	return pkgctx.SystemAccountID
}

// PopulateEvent fills a fresh audit_event builder. id must already be allocated.
// Caller owns retry (builder.SetID + Build again).
func PopulateEvent(
	builder instance_builders.InstanceBuilder,
	id, projectRoot, actor, createdAt string,
	options *EventOptions,
) {
	if builder == nil || options == nil {
		return
	}
	builder.SetID(id)
	builder.SetStatus(string(audit_event.StatusCompleted))
	builder.SetField(FieldOriginSystem, validation.DefaultOriginSystem)
	builder.SetField(FieldOriginProject, validation.DefaultOriginProject)
	builder.SetField(FieldNamespaceID, validation.DefaultNamespaceKernel)
	builder.SetField(FieldEventType, options.EventType)
	builder.SetField(FieldOperation, options.Operation)
	builder.SetField(FieldSeverity, options.Severity)
	if options.Operation != "" {
		builder.SetField(FieldTitle, options.Operation)
	}
	builder.SetField(FieldCreatedAt, createdAt)
	builder.SetField(FieldCreatedBy, actor)
	updatedAt := options.UpdatedAt
	if updatedAt == "" {
		updatedAt = createdAt
	}
	builder.SetField(FieldUpdatedAt, updatedAt)
	builder.SetField(FieldUpdatedBy, actor)
	if options.TargetKind != "" {
		builder.SetField(FieldTargetKind, options.TargetKind)
	}
	if options.TargetID != "" {
		builder.SetField(FieldTargetID, options.TargetID)
	}
	if options.TargetPath != "" {
		relPath, relErr := filepath.Rel(projectRoot, options.TargetPath)
		if relErr != nil {
			relPath = options.TargetPath
		}
		builder.SetField(FieldTargetPath, relPath)
	}
	if len(options.Metadata) > 0 {
		builder.SetField(FieldMetadata, options.Metadata)
	}
	if options.OriginalValue != "" {
		builder.SetField(FieldOriginalValue, options.OriginalValue)
	}
	if options.NewValue != "" {
		builder.SetField(FieldNewValue, options.NewValue)
	}
	if options.RecoveryMethod != "" {
		builder.SetField(FieldRecoveryMethod, options.RecoveryMethod)
	}
	if options.Reason != nil {
		builder.SetField(FieldReason, options.Reason)
	}
	if options.SessionID != "" {
		builder.SetField(FieldSessionID, options.SessionID)
	}
	if options.OccurrenceCount > 0 {
		builder.SetField(FieldOccurrenceCnt, options.OccurrenceCount)
	}
	if len(options.OccurrenceTimestamps) > 0 {
		builder.SetField(FieldOccurrenceTs, options.OccurrenceTimestamps)
	}
}

// NewEventBuilder returns a fresh audit_event builder for the spec's schema version.
func NewEventBuilder() (instance_builders.InstanceBuilder, error) {
	schemaVersion, err := instance_builders.SchemaVersionForKind(Kind)
	if err != nil {
		return nil, err
	}
	return instance_builders.NewForKind(Kind, schemaVersion), nil
}

// BuildEventMap constructs an audit_event instance map. id must already be allocated.
func BuildEventMap(
	id, projectRoot string,
	secCtx *pkgctx.SecurityContext,
	options *EventOptions,
	now time.Time,
	allowed map[string]struct{},
	fallback string,
) (map[string]any, error) {
	if options == nil {
		return nil, nil
	}
	actor := ResolveActor(secCtx, options.CreatedBy)
	eventType, metadata := NormalizeEventType(options.EventType, options.Metadata, allowed, fallback)
	createdAt := options.CreatedAt
	if createdAt == "" {
		createdAt = now.UTC().Format(time.RFC3339)
	}
	builder, err := NewEventBuilder()
	if err != nil {
		return nil, err
	}
	filled := *options
	filled.EventType = eventType
	filled.Metadata = metadata
	PopulateEvent(builder, id, projectRoot, actor, createdAt, &filled)
	return builder.Build()
}

// BufferCheckMap is the minimal map used to decide aggregation before a full Build.
func BufferCheckMap(options *EventOptions) map[string]any {
	if options == nil {
		return nil
	}
	m := map[string]any{
		FieldEventType: options.EventType,
		FieldSeverity:  options.Severity,
	}
	if options.TargetKind != "" {
		m[FieldTargetKind] = options.TargetKind
	}
	if options.TargetID != "" {
		m[FieldTargetID] = options.TargetID
	}
	if options.Metadata != nil {
		m[FieldMetadata] = options.Metadata
	}
	return m
}
