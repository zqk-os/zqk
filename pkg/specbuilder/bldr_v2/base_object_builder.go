package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// BaseObjectBuilder builds the base_object spec at version v2_0_0
// File: bldr_v2/base_object_builder.go - version is encoded in package/directory name
type BaseObjectBuilder struct {
	*builders.BaseSpecBuilder
}

// NewBaseObjectBuilder creates a new builder for base_object spec version v2_0_0
func NewBaseObjectBuilder() *BaseObjectBuilder {
	builder := &BaseObjectBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("base_object", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("auditable").
		SetDescription("Core structural fields shared by every system object after audit metadata. Objects extending base_object use the default base_lifecycle.yaml lifecycle unless they define their own lifecycle file. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addBaseObjectFields()

	return builder
}

// addBaseObjectFields adds the base_object fields
func (b *BaseObjectBuilder) addBaseObjectFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("actual_effort", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation").
			AutomationHooks("used for velocity calculations and estimation accuracy analysis.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("time tracking, metrics collection.").
			Lifecycle("mutable (updated as work progresses)").
			Observability("yes").
			Purpose("Actual effort expended (e.g., \\\\\\\"5.2 days\\\\\\\", \\\\\\\"32 hours\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"metrics",
				"retrospectives",
			}).
			Validation("Free-form string describing actual effort.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("answer_due_by", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("question asker/owner").
			AutomationHooks("triggers reminders for unanswered questions.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("question tracking, notification system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("ISO-8601 date for when a question needs an answer.").
			Security("non-sensitive").
			SystemUsage([]any{
				"question tracking",
				"notifications",
				"accountability",
			}).
			Validation("ISO-8601 date format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}$`).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-017"))
	b.AddFieldBuilder(builders.NewFieldBuilder("artifacts", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation").
			AutomationHooks("ensures watchers update those files.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("doc index updates.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Files/components touched by this object.").
			Security("non-sensitive").
			SystemUsage([]any{
				"doc index",
				"dependency mapping",
			}).
			Validation("paths must exist or be declared.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("completeness_validation", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system/automation").
			AutomationHooks("used by orchestrators to rigorously verify object completeness before advancing its lifecycle.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("verification engine.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("A programmatic Universal Verification DSL definition dictating exactly how this object's completeness is objectively verified.").
			Security("non-sensitive").
			SystemUsage([]any{
				"state_transitions",
				"objective_verification",
			}).
			Validation("List of Validation DSL step configurations.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("BSE-021"))
	b.AddFieldBuilder(builders.NewFieldBuilder("context", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive depending on goal authority.").
			AutomationHooks("used to seed prompts.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("prompt templates, decision records.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Problem statement / motivation.").
			Security("may contain sensitive details; treat as confidential by default.").
			SystemUsage([]any{
				"reasoning",
				"prompting",
			}).
			Validation("markdown allowed; must mention goal/workstream when applicable.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("deadline", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive").
			AutomationHooks("triggers deadline reminders and priority escalation.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduler, notification system.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("ISO-8601 date or datetime when the object must be completed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduling",
				"notifications",
				"priority calculation",
			}).
			Validation("ISO-8601 date or datetime format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}(T\d{2}:\d{2}:\d{2}Z)?$`).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("dependencies", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used during delete/merge operations.").
			Cardinality("many (can be empty)").
			Criticality("composition").
			Default([]any{}).
			Dependencies("linkage constraints.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Related object IDs this entity depends on.").
			Security("non-sensitive").
			SystemUsage([]any{
				"graph validity",
				"impact analysis",
			}).
			Validation("referenced IDs must exist or be placeholders.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("estimated_effort", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/planner").
			AutomationHooks("used for timeline projections and capacity planning.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduling, timeline generation.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Estimated effort required (e.g., \\\\\\\"4-6 weeks\\\\\\\", \\\\\\\"2 days\\\\\\\", \\\\\\\"8 hours\\\\\\\", \\\\\\\"5 story points\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"scheduling",
				"resource allocation",
			}).
			Validation("Free-form string describing effort estimate.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (Generator)").
			AutomationHooks("used for cross-file references.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-assigned per kind sequence").
			Dependencies("linkage constraints, URN creation.").
			Lifecycle("immutable").
			Observability("logged + manifests.").
			Purpose("Stable identifier used across the graph (\\\\\\\"PREFIX-####\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
				"URIs",
			}).
			Validation("regex ^[A-Z]+-\\\\\\\\d{3,}$; uniqueness enforced.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(2048).
			Pattern(`^[A-Z]+-\d{3,}$`).
			Required(true).
			Build()).
		WithTraits("field_queryable_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("BSE-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("kind", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation").
			AutomationHooks("drives validator selection.").
			Cardinality("one").
			Criticality("composition").
			Default("derived from generator template").
			Dependencies("schema selection.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Declares ontology type (decision, requirement, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
				"validation",
			}).
			Validation("must match registered kinds.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("BSE-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("namespace_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/system").
			AutomationHooks("automatically derived from object ID or kind registry").
			Cardinality("one").
			Criticality("association").
			Default("derived from ID or kind registry").
			Dependencies("namespace registry, ID validator").
			Lifecycle("immutable (derived, not user-set)").
			Observability("yes").
			Purpose("Namespace identifier for this object (e.g., \\\\\\\"zqk:kernel\\\\\\\", \\\\\\\"domain:organizational\\\\\\\")").
			Security("non-sensitive").
			SystemUsage([]any{
				"namespace-aware queries",
				"cross-namespace reference validation",
				"namespace isolation",
			}).
			Validation("Namespace ID format pattern (zqk:kernel, domain:*, integration:*)").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^(zqk|domain|integration):[a-z0-9_]+(:[a-z0-9_]+)*$`).
			Required(false).
			Build()).
		WithTraits("field_read_only_group", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("BSE-019"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority_tier", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive").
			AutomationHooks("used for priority-based filtering and sorting in reports.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("priority plan, backlog management.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Priority tier tracking (P0/P1/P2/P3) for prioritization.").
			Security("non-sensitive").
			SystemUsage([]any{
				"prioritization",
				"planning",
				"filtering",
			}).
			Validation("enum (\\\\\\\"P0\\\\\\\", \\\\\\\"P1\\\\\\\", \\\\\\\"P2\\\\\\\", \\\\\\\"P3\\\\\\\") where P0 is critical, P1 is high, P2 is medium, P3 is low.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"P0",
				"P1",
				"P2",
				"P3",
			}).
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-016"))
	b.AddFieldBuilder(builders.NewFieldBuilder("questions", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (generator)").
			AutomationHooks("used to re-run prompts.").
			Cardinality("many").
			Criticality("association").
			Default("filled during creation flow").
			Dependencies("template regeneration.").
			Lifecycle("append-only per iteration").
			Observability("available for audits.").
			Purpose("Prompt items that captured this object's data.").
			Security("may include sensitive context; mark as confidential.").
			SystemUsage([]any{
				"regeneration",
				"explanations",
			}).
			Validation("stored as structured Q/A pairs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("BSE-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("related_object_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation").
			AutomationHooks("optional graph expansion, cross-plan and secondary traceability reports.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("object ID registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional links to other objects by stable ID (any kind). Use when typed *\\\\_refs fields are not enough\n(e.g. secondary priority plans, historical milestones, parallel tracks). Distinct from dependencies\n(structural depends-on for impact analysis). Empty list when unused.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"reporting",
			}).
			Validation("each entry is a valid object ID; targets may be validated when read.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("BSE-020"))
	b.AddFieldBuilder(builders.NewFieldBuilder("schema_version", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/importer.").
			AutomationHooks("ensures correct validator is loaded.").
			Cardinality("one").
			Criticality("composition").
			Default("current spec version at creation").
			Dependencies("import/export tooling.").
			Lifecycle("immutable except when migrations run.").
			Observability("yes").
			Purpose("SemVer indicating which object schema this instance conforms to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"imports",
				"validation",
			}).
			Validation("SemVer pattern (e.g., 1.0.0 or 2.0.0).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d+\.\d+\.\d+$`).
			Required(true).
			Build()).
		WithTraits("field_read_only_group", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("BSE-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/importer.").
			AutomationHooks("determines how strictly to enforce validation/fallbacks.").
			Cardinality("one").
			Criticality("composition").
			Default("internal").
			Dependencies("fallback logic (e.g., prefer internal if external fails).").
			Lifecycle("immutable unless re-imported.").
			Observability("yes").
			Purpose("Indicates if the object originated internally, was imported, or provided by a user.").
			Security("non-sensitive").
			SystemUsage([]any{
				"resolver decisions",
				"trust policies",
			}).
			Validation("enum (\\\\\\\"internal\\\\\\\", \\\\\\\"external\\\\\\\", \\\\\\\"imported\\\\\\\"); configs may extend.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"internal",
				"external",
				"imported",
			}).
			Required(false).
			Build()).
		WithTraits("field_read_only_group", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("BSE-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("spec_adherence", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/validator.").
			AutomationHooks("prevents imports/promotions if adherence is \"fail\".").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{
				"status": "unknown",
			}).
			Dependencies("validator tooling, import pipeline.").
			Lifecycle("mutable (updated whenever validation runs).").
			Observability("yes").
			Purpose("Records the validation status/results for the current schema version.").
			Security("non-sensitive").
			SystemUsage([]any{
				"imports",
				"audits",
			}).
			Validation("Structured object containing fields like \\\\\\\"status\\\\\\\" (pass/fail), \\\\\\\"timestamp\\\\\\\", \\\\\\\"validator_fingerprint\\\\\\\".").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_read_only_group").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("BSE-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("stakeholders", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used to mention watchers.").
			Cardinality("many (>=0)").
			Criticality("association").
			Default([]any{}).
			Dependencies("scheduling, approvals.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Personas/roles impacted.").
			Security("no direct PII (role names only).").
			SystemUsage([]any{
				"routing",
				"notifications",
			}).
			Validation("entries from controlled vocab.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("depends_on_role (owner/executive)").
			AutomationHooks("triggers notifications/testing when status changes.").
			Cardinality("one").
			Criticality("composition").
			Default("proposed").
			Dependencies("scheduler, enforcement.").
			Lifecycle("mutable (track via auditable fields)").
			Observability("yes").
			Purpose("Lifecycle stage (\\\\\\\"proposed\\\\\\\", \\\\\\\"approved\\\\\\\", \\\\\\\"in_progress\\\\\\\", \\\\\\\"implemented\\\\\\\", \\\\\\\"archived\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"workflows",
				"reports",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "access:system_override").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("listable", "readable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("r--").
		WithSemanticType("statement").
		WithProfileCode("BSE-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status_history", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/lifecycle system").
			AutomationHooks("automatically populated on status transitions; used for audit trails and lifecycle analysis.").
			Cardinality("many (>=0)").
			Criticality("association").
			Default([]any{}).
			Dependencies("lifecycle system, status transition logic.").
			Lifecycle("append-only (new entries added on status changes)").
			Observability("yes").
			Purpose("List of StatusHistoryEntry records for auditing and lifecycle analysis.").
			Security("non-sensitive (may contain actor information).").
			SystemUsage([]any{
				"auditing",
				"lifecycle analysis",
				"status transition tracking",
			}).
			Validation("List of StatusHistoryEntry objects with fields: sequence (int), timestamp (string), status (string), note/notes (string), actor/updated_by (string), updated_at (string). Supports both Workstream format (sequence, timestamp, status, note) and BacklogItem format (status, updated_at, updated_by, notes).").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_read_only_group").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("BSE-018"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_date", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/planner").
			AutomationHooks("used for timeline visualization and milestone tracking.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("timeline generation, reporting.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("ISO-8601 date (alternative to deadline) for target completion.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"reporting",
			}).
			Validation("ISO-8601 date format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}$`).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-015"))
	b.AddFieldBuilder(builders.NewFieldBuilder("title", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner role").
			AutomationHooks("used in generated docs.").
			Cardinality("one").
			Criticality("composition").
			Default("required at creation").
			Dependencies("doc index, reports.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Human-readable name/summary.").
			Security("non-sensitive").
			SystemUsage([]any{
				"UI display",
				"search",
			}).
			Validation("5–120 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(120).
			MinLength(5).
			Pattern(`^.+$`).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-002"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *BaseObjectBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *BaseObjectBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *BaseObjectBuilder) GetOntology() string {
	return "base_object"
}

func init() {
	builders.RegisterBuilder(NewBaseObjectBuilder())
}
