package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// AuditEventBuilder builds the audit_event spec at version v2_0_0
// File: bldr_v2/audit_event_builder.go - version is encoded in package/directory name
type AuditEventBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAuditEventBuilder creates a new builder for audit_event spec version v2_0_0
func NewAuditEventBuilder() *AuditEventBuilder {
	builder := &AuditEventBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("audit_event", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Audit events record security-sensitive operations and integrity-related actions for accountability, compliance, and security analysis. These events are immutable and provide a complete audit trail of who performed what operation, when, and why.\\nLifecycle: audit_event_lifecycle.yaml (pending, completed, failed, reverted, archived, error). Status allowed values and transitions are defined there; do not duplicate in this spec.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("read_only_group")

	// Add fields
	builder.addAuditEventFields()

	return builder
}

// addAuditEventFields adds the audit_event fields
func (b *AuditEventBuilder) addAuditEventFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("aggregated_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used to calculate aggregation efficiency and volume metrics.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (only for aggregated_summary events)").
			Dependencies("only present when event_type is \"aggregated_summary\"").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Total number of individual events aggregated into this summary event.").
			Security("non-sensitive").
			SystemUsage([]any{
				"aggregation reporting",
				"volume analysis",
			}).
			Validation("Must be >= 1 if present.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("measurement").
		WithProfileCode("AUE-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("aggregated_events", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used to provide detailed breakdown of aggregated events.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (only for aggregated_summary events)").
			Dependencies("only present when event_type is \"aggregated_summary\"").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Array of event summaries with counts for each event type/kind combination aggregated.").
			Security("non-sensitive").
			SystemUsage([]any{
				"aggregation reporting",
				"detailed breakdown",
			}).
			Validation("Array of objects with event_type, count, target_kind (optional), time_range.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-015"))
	b.AddFieldBuilder(builders.NewFieldBuilder("aggregation_window", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used to query aggregated events by time window.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (only for aggregated_summary events)").
			Dependencies("only present when event_type is \"aggregated_summary\"").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Time range covered by aggregated events (ISO 8601 interval format, e.g., \\\\\\\"2025-12-25T14:00:00Z to 2025-12-25T15:00:00Z\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"aggregation reporting",
				"time range queries",
			}).
			Validation("ISO 8601 interval format or custom \\\\\\\"from to\\\\\\\" format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "searchable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("event_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for event routing, alerting, reporting.").
			Cardinality("one").
			Criticality("composition").
			Default("required - must be specified").
			Dependencies("event-specific fields may be required based on type.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of audit event (hash_mismatch_fix, hash_regeneration, integrity_recovery, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"reporting",
				"security analysis",
			}).
			Validation("Must be one of defined event types.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"hash_mismatch_fix",
				"hash_regeneration",
				"integrity_recovery",
				"object_deletion",
				"object_quarantine",
				"object_unquarantine",
				"object_creation",
				"object_update",
				"object_move",
				"permission_change",
				"role_assignment",
				"security_alert",
				"system_config_change",
				"cache_invalidation",
				"cache_update",
				"cache_bulk_invalidation",
				"cache_operation",
				"cache_availability",
				"cache_cleanup",
				"aggregated_summary",
				"command_execution",
				"code_quality_bypass",
				"scheduler_job_started",
				"scheduler_job_completed",
				"scheduler_job_failed",
				"listing_index_batch_start",
				"listing_index_batch_complete",
				"listing_index_batch_error",
				"hash_registry_batch_start",
				"hash_registry_batch_complete",
				"hash_registry_batch_error",
				"validation_start",
				"validation_complete",
				"validation_error",
				"async_router_start",
				"async_router_complete",
				"async_router_error",
				"operation_executor_start",
				"operation_executor_complete",
				"operation_executor_error",
				"change_journal_created",
				"change_journal_error",
				"audit_buffer_flush_start",
				"audit_buffer_flush_complete",
				"audit_buffer_flush_error",
				"orphan_cleanup_start",
				"orphan_cleanup_complete",
				"orphan_cleanup_error",
				"service_start",
				"service_stop",
				"service_error",
				"migration_start",
				"migration_complete",
				"migration_error",
				"scenario_builder_start",
				"scenario_builder_progress",
				"scenario_builder_complete",
				"scenario_builder_error",
				"prompt_run_start",
				"prompt_run_complete",
				"prompt_run_error",
			}).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (Generator)").
			AutomationHooks("used for cross-file references in audit trail.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-assigned per sequence").
			Dependencies("linkage constraints, audit trail.").
			Lifecycle("immutable").
			Observability("logged + manifests.").
			Purpose("Stable identifier for audit event (\\\\\\\"AUD-####\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
				"audit trail",
			}).
			Validation("regex ^AUD-\\\\\\\\d+$ (allows 1+ digits, unlike base_object which requires 3+)").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^AUD-\d+$`).
			Required(true).
			Build()).
		WithTraits("field_queryable_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-ID"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metadata", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for detailed forensic analysis.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (optional)").
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Additional structured metadata about the event (command flags, context, environment, etc.).").
			Security("may contain sensitive data (user info, IP addresses, etc.) - respect masking rules").
			SystemUsage([]any{
				"forensics",
				"debugging",
				"audit reports",
			}).
			Validation("Free-form object (key-value pairs).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("AUE-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("new_value", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for verification and audit reports.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null if not applicable to event type").
			Dependencies("event_type determines value format").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("New value after the operation (e.g., new hash, new status).").
			Security("non-sensitive (but may contain sensitive data like hashes)").
			SystemUsage([]any{
				"audit reports",
				"verification",
			}).
			Validation("Value depends on event_type.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("occurrence_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used to track repeated operations in bulk contexts, similar to status_history append pattern.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (assumed to be 1 if not present)").
			Dependencies("only present when event occurred multiple times in a bulk operation").
			Lifecycle("mutable (incremented during bulk operations, append-only pattern)").
			Observability("yes").
			Purpose("Number of times this event occurred (for bulk operations). Defaults to 1 if not present.").
			Security("non-sensitive").
			SystemUsage([]any{
				"bulk operation tracking",
				"frequency analysis",
				"reporting",
			}).
			Validation("Must be >= 1 if present.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("quantity").
		WithProfileCode("AUE-017"))
	b.AddFieldBuilder(builders.NewFieldBuilder("occurrence_timestamps", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used to track when repeated operations occurred, enables temporal analysis of bulk operations.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (assumed to be [created_at] if not present)").
			Dependencies("only present when occurrence_count > 1").
			Lifecycle("mutable (append-only, new timestamps added, never removed)").
			Observability("yes").
			Purpose("List of timestamps when this event occurred (for bulk operations). Append-only list similar to status_history.").
			Security("non-sensitive").
			SystemUsage([]any{
				"temporal analysis",
				"bulk operation tracking",
				"audit trail",
			}).
			Validation("List of ISO 8601 timestamps in UTC (YYYY-MM-DDTHH:MM:SSZ format).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-018"))
	b.AddFieldBuilder(builders.NewFieldBuilder("operation", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used in audit reports and summaries.").
			Cardinality("one").
			Criticality("composition").
			Default("required - must be specified").
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Human-readable description of the operation performed (e.g., \\\\\\\"Regenerated hash for BLI-626\\\\\\\", \\\\\\\"Recovered integrity for priority_plan\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"audit reports",
				"human-readable logs",
			}).
			Validation("Free-form string describing the operation.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinLength(1).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("original_value", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for rollback and forensic analysis.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null if not applicable to event type").
			Dependencies("event_type determines value format").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Original value before the operation (e.g., original hash, original status).").
			Security("non-sensitive (but may contain sensitive data like hashes)").
			SystemUsage([]any{
				"rollback",
				"audit reports",
				"forensics",
			}).
			Validation("Value depends on event_type (hash for hash operations, status for status changes, etc.).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("preserved_samples", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for forensic analysis of aggregated events.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (samples only preserved if configured)").
			Dependencies("only present when event_type is \"aggregated_summary\" and samples were preserved").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Array of sample individual events preserved for forensics (if configured to preserve samples).").
			Security("non-sensitive").
			SystemUsage([]any{
				"forensics",
				"detailed analysis",
			}).
			Validation("Array of event objects (full or partial).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-016"))
	b.AddFieldBuilder(builders.NewFieldBuilder("reason", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("user/automation").
			AutomationHooks("used in audit reports to explain actions.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (optional)").
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Optional explanation of why the operation was performed (user-provided or system-generated).").
			Security("non-sensitive").
			SystemUsage([]any{
				"audit reports",
				"justification",
			}).
			Validation("Free-form text.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("recovery_method", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used to analyze recovery patterns and identify process gaps.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null if not applicable").
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Method used to perform the operation (auto-fix, manual, force, interactive, scheduled).").
			Security("non-sensitive").
			SystemUsage([]any{
				"audit reports",
				"pattern analysis",
			}).
			Validation("Must be one of defined methods.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"auto-fix",
				"manual",
				"force",
				"interactive",
				"scheduled",
			}).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "groupable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("severity", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for alerting and security monitoring.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("determined by event_type (hash_mismatch_fix = high, hash_regeneration = medium, etc.)").
			Dependencies("event_type may determine default severity").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Severity level of the event (low, medium, high, critical) for security analysis.").
			Security("non-sensitive").
			SystemUsage([]any{
				"alerting",
				"prioritization",
				"reporting",
			}).
			Validation("Must be one of defined severity levels.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"low",
				"medium",
				"high",
				"critical",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for lifecycle tracking and reporting.").
			Cardinality("one").
			Criticality("composition").
			Default("completed").
			Dependencies("lifecycle registry (see audit_event_lifecycle.yaml)").
			Lifecycle("mutable (typically transitions: pending -> completed/failed, completed -> reverted)").
			Observability("yes").
			Purpose("Status of the audit event operation; allowed values and transitions from audit_event_lifecycle.yaml.").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle",
				"reporting",
			}).
			Validation("Allowed values from lifecycle; do not duplicate enum here.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("AUE-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used to link events to specific objects.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null if event doesn't target a specific object").
			Dependencies("object registry").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ID of the specific object that was affected (e.g., \\\\\\\"BLI-626\\\\\\\", \\\\\\\"PRI-208\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"audit reports",
			}).
			Validation("Must reference existing object if provided.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "listable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("AUE-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_kind", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for kind-based filtering and reporting.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null if event doesn't target a specific object kind").
			Dependencies("object registry").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Object kind that was affected (e.g., \\\\\\\"backlog_item\\\\\\\", \\\\\\\"priority_plan\\\\\\\", \\\\\\\"requirement\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"reporting",
			}).
			Validation("Must match known object kind.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_path", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for file-level audit reports.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null if event doesn't target a specific file").
			Dependencies("file system").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("File path that was affected (e.g., \\\\\\\"\\.zqk/process/backlog_items/BLI-626.yaml\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"file-level auditing",
			}).
			Validation("Must be a valid file path relative to project root.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "searchable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("title", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("used for display purposes.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (operation field is primary identifier)").
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Optional human-readable title for the audit event (operation field is primary identifier).").
			Security("non-sensitive").
			SystemUsage([]any{
				"display",
				"filtering",
			}).
			Validation("Free-form string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AUE-000"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AuditEventBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AuditEventBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AuditEventBuilder) GetOntology() string {
	return "audit_event"
}

func init() {
	builders.RegisterBuilder(NewAuditEventBuilder())
}
