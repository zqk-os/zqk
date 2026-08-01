package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// PriorityPlanBuilder builds the priority_plan spec at version v2_0_0
// File: bldr_v2/priority_plan_builder.go - version is encoded in package/directory name
type PriorityPlanBuilder struct {
	*builders.BaseSpecBuilder
}

// NewPriorityPlanBuilder creates a new builder for priority_plan spec version v2_0_0
func NewPriorityPlanBuilder() *PriorityPlanBuilder {
	builder := &PriorityPlanBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("priority_plan", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a priority plan that organizes backlog items into priority tiers (P0, P1, P2, P3). Priority plans are used to sequence work and manage capacity across workstreams.\\nNEW MODEL (as of 2025-12-15): Priority plans no longer store tier arrays (p0_items, p1_items, etc.). Instead, backlog items reference priority plans via their priority_plan_ref field, and tiering comes from each item's priority_tier field. This ensures tiering is driven from backlog items rather than being arbitrarily placed into buckets in the plan. To find items for a priority plan, query: \\\"SELECT backlog_items WHERE priority_plan_ref = plan_id\\\"; the tiering for each item comes from the item's priority_tier field (P0, P1, P2, P3).\\nLifecycle: priority_plan_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("constrainable")

	// Add fields
	builder.addPriorityPlanFields()

	return builder
}

// addPriorityPlanFields adds the priority_plan fields
func (b *PriorityPlanBuilder) addPriorityPlanFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("active_order", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for deterministic current plan selection when multiple plans are active.").
			Cardinality("one").
			Criticality("metadata").
			Default("null (treated as lowest priority)").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Explicit ordering for active priority plans. Lower numbers take precedence when multiple plans are active. If not set, defaults to lowest priority (treated as highest number).").
			Security("non-sensitive").
			SystemUsage([]any{
				"selection",
				"ordering",
			}).
			Validation("Must be a positive integer. Lower numbers indicate higher priority for current plan selection.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("ordering").
		WithProfileCode("PRIO-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in reports and documentation.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Description of this priority plan.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"context",
			}).
			Validation("Free text or markdown.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PRIO-000"))
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
			Purpose("Stable identifier for the priority plan. Supports formats like \\\\\\\"PRIO-20251212-week1\\\\\\\", \\\\\\\"PRIO-4\\\\\\\", \\\\\\\"PLAN-001\\\\\\\", etc.").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
				"URIs",
			}).
			Validation("Must start with PLAN- or PRIO- followed by alphanumeric characters, dots, dashes, or underscores.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^(PLAN-|PRIO-)[A-Za-z0-9][A-Za-z0-9._-]*$`).
			Required(true).
			Build()).
		WithTraits("filterable", "listable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("PRIO-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("next_plan_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("used for plan history navigation.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("priority_plan registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to the next priority plan for historical tracking.").
			Security("non-sensitive").
			SystemUsage([]any{
				"tracking",
				"navigation",
			}).
			Validation("Must reference existing priority plan ID.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("PRIO-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("note", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in reports and documentation.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional notes about the priority plan.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"context",
			}).
			Validation("Free text or markdown.").
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
		WithProfileCode("PRIO-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("persona_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for persona-based filtering.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("persona registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of persona IDs associated with this priority plan.").
			Security("non-sensitive").
			SystemUsage([]any{
				"grouping",
				"reporting",
				"routing",
			}).
			Validation("Must reference existing persona IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("PRIO-015"))
	b.AddFieldBuilder(builders.NewFieldBuilder("plan_date", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("used for plan date tracking.").
			Cardinality("one").
			Criticality("metadata").
			Default(nil).
			Dependencies("none.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO-8601 date when this priority plan was created.").
			Security("non-sensitive").
			SystemUsage([]any{
				"tracking",
				"reporting",
			}).
			Validation("Must be in ISO-8601 format (YYYY-MM-DD or RFC3339).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("date").
		WithProfileCode("PRIO-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("plan_version", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for plan versioning.").
			Cardinality("one").
			Criticality("metadata").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Version identifier for this priority plan (e.g., \\\\\\\"v1.0\\\\\\\", \\\\\\\"2025-01-21\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"tracking",
				"versioning",
			}).
			Validation("Free text version identifier.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("version").
		WithProfileCode("PRIO-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("previous_plan_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("used for plan history navigation.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("priority_plan registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to the previous priority plan for historical tracking.").
			Security("non-sensitive").
			SystemUsage([]any{
				"tracking",
				"navigation",
			}).
			Validation("Must reference existing priority plan ID.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("PRIO-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("rationale", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used in reports and documentation.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Rationale for priority assignments in this plan.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"context",
			}).
			Validation("Free text or markdown.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PRIO-000"))
	b.AddFieldBuilder(builders.NewFieldBuilder("release_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("used for release tracking.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("release registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Link to release object created when plan completes.").
			Security("non-sensitive").
			SystemUsage([]any{
				"tracking",
				"reporting",
			}).
			Validation("Must reference existing release ID.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("PRIO-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source_file", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("used for plan generation tracking.").
			Cardinality("one").
			Criticality("metadata").
			Default(nil).
			Dependencies("none.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Path to source markdown file used to create this priority plan.").
			Security("non-sensitive").
			SystemUsage([]any{
				"metadata",
				"tracking",
			}).
			Validation("File path string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("path").
		WithProfileCode("PRIO-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source_format", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("used for plan generation tracking.").
			Cardinality("one").
			Criticality("metadata").
			Default("manual").
			Dependencies("none.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Indicates the source format used to create this priority plan.").
			Security("non-sensitive").
			SystemUsage([]any{
				"metadata",
				"tracking",
			}).
			Validation("Must be one of: markdown, yaml, manual.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"markdown",
				"yaml",
				"manual",
			}).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("metadata").
		WithProfileCode("PRIO-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("team_configuration_ref", "reference").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner").
			AutomationHooks("used by CAP Loop sub-kernel provisioner").
			Cardinality("zero_or_one").
			Criticality("metadata").
			Default(nil).
			Dependencies("team configuration registry").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References a team configuration (cellular archetype) for the priority plan pod execution.").
			Security("non-sensitive").
			SystemUsage([]any{
				"orchestrator provisioning",
			}).
			Validation("Must reference a team_configuration ID").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "modifiable", "readable", "searchable", "writable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("PRIO-018"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workflow_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("used for workflow constraint validation during activation.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("workflow registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to workflow object that defines constraints and rules for this priority plan. Workflow constraints are enforced when plan transitions to active status.").
			Security("non-sensitive").
			SystemUsage([]any{
				"workflow validation",
				"constraint enforcement",
				"role-based access control",
			}).
			Validation("Must reference existing workflow object ID (WFL-###).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("PRIO-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for workstream-based grouping.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Link to associated workstream (singular reference, legacy field).").
			Security("non-sensitive").
			SystemUsage([]any{
				"grouping",
				"reporting",
			}).
			Validation("Must reference existing workstream ID.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("PRIO-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for workstream-based grouping.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of workstream IDs associated with this priority plan.").
			Security("non-sensitive").
			SystemUsage([]any{
				"grouping",
				"reporting",
			}).
			Validation("Must reference existing workstream IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("PRIO-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *PriorityPlanBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *PriorityPlanBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *PriorityPlanBuilder) GetOntology() string {
	return "priority_plan"
}

func init() {
	builders.RegisterBuilder(NewPriorityPlanBuilder())
}
