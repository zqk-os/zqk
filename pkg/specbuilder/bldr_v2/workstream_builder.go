package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// WorkstreamBuilder builds the workstream spec at version v2_0_0
// File: bldr_v2/workstream_builder.go - version is encoded in package/directory name
type WorkstreamBuilder struct {
	*builders.BaseSpecBuilder
}

// NewWorkstreamBuilder creates a new builder for workstream spec version v2_0_0
func NewWorkstreamBuilder() *WorkstreamBuilder {
	builder := &WorkstreamBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("workstream", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("work_interval").
		SetDescription("Full workstream entries (mirrors \\\"docs/planning/workstreams.json\\\"). Tracks active and completed bodies of work with ordering, status history, and related artifacts.\\nLifecycle: workstream_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable").
		AddTrait("completable")

	// Add fields
	builder.addWorkstreamFields()

	return builder
}

// addWorkstreamFields adds the workstream fields
func (b *WorkstreamBuilder) addWorkstreamFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("application", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("filtering.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("dashboards.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional application/component tag.").
			Security("non-sensitive").
			SystemUsage([]any{
				"distinguish tangential items",
			}).
			Validation("<= 80 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(80).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WKS-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("blockers", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("triggers alerts, prevents status transitions.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Known blockers preventing progress on this workstream.").
			Security("non-sensitive").
			SystemUsage([]any{
				"status tracking",
				"dependency management",
				"reporting",
			}).
			Validation("list of strings describing blockers.").
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
		WithProfileCode("WKS-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("category", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used to reorder/cluster.").
			Cardinality("one").
			Criticality("composition").
			Default("feature").
			Dependencies("dashboards.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Scope classification (application/system/feature/component/ops/tooling/etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"grouping",
				"filters",
			}).
			Validation("enum; extend via config if needed.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"application",
				"system",
				"feature",
				"component",
				"ops",
				"tooling",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WKS-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in reports, documentation.").
			Cardinality("one").
			Criticality("association").
			Default("none (optional, prefer context for brief description)").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Detailed description of the workstream's purpose and scope.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"communication",
				"understanding",
			}).
			Validation("free text or markdown.").
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
		WithProfileCode("WKS-016"))
	b.AddFieldBuilder(builders.NewFieldBuilder("entry_point", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("doc index regeneration.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("doc index.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Canonical doc or script describing the workstream.").
			Security("non-sensitive").
			SystemUsage([]any{
				"navigation",
			}).
			Validation("path/URL must exist.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WKS-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metadata", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation.").
			AutomationHooks("e.g., \"has_deferred_items\" used by CLI.").
			Cardinality("one").
			Criticality("association").
			Default(map[string]any{}).
			Dependencies("CLI metadata filters.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Additional flags (has_deferred_items, clarifications, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filters",
			}).
			Validation("JSON object with documented keys.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("WKS-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("order", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("ensures consistent display.").
			Cardinality("one").
			Criticality("composition").
			Default("sequential").
			Dependencies("CLI reorder.").
			Lifecycle("mutable (only for active statuses).").
			Observability("yes").
			Purpose("Ordering among active workstreams.").
			Security("non-sensitive").
			SystemUsage([]any{
				"prioritization",
			}).
			Validation("positive integer.").
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
		WithProfileCode("WKS-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("owner_display", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("none.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("dashboards.").
			Lifecycle("mutable (kept in sync with account).").
			Observability("yes").
			Purpose("Cached human-readable owner name.").
			Security("PII—confidential.").
			SystemUsage([]any{
				"UI display",
			}).
			Validation("<= 80 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(80).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WKS-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("owner_ref", "reference").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive/admin.").
			AutomationHooks("ensures ownership is validated.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("authorization checks.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to account/role representing the owner.").
			Security("may contain PII—treat as confidential.").
			SystemUsage([]any{
				"permissions",
				"reporting",
			}).
			Validation("URI or structured reference (\\\\\\\"account:{id}\\\\\\\") pointing to an existing account/role.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WKS-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("prerequisites", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("prevents progression until prerequisites complete.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("gating logic.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Other workstreams, milestones, or conditions that must be complete before this workstream can start.").
			Security("non-sensitive").
			SystemUsage([]any{
				"dependency graph",
				"gating logic",
			}).
			Validation("references to workstreams, milestones, or other objects.").
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
		WithProfileCode("WKS-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("related_docs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("ensures doc index coverage.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("doc index.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Supporting docs/information.").
			Security("non-sensitive").
			SystemUsage([]any{
				"doc index",
			}).
			Validation("paths must exist or be declared.").
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
		WithProfileCode("WKS-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("stage_type", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for filtering, stage-based reporting.").
			Cardinality("one").
			Criticality("association").
			Default("none (optional)").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Stage or phase classification for this workstream (e.g., \\\\\\\"planning\\\\\\\", \\\\\\\"execution\\\\\\\", \\\\\\\"validation\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"filtering",
				"reporting",
			}).
			Validation("free text classification.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "groupable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WKS-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workflow_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/admin.").
			AutomationHooks("used for workflow constraint validation during workstream operations.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("workflow registry, priority plan workflow.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to workflow object that defines constraints and rules for this workstream. If not set, inherits workflow from associated priority plan. Workflow constraints enforce role-based object creation/manipulation restrictions.").
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
		WithTraits("filterable", "modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("WKS-018"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for dependency graphs, related workstream discovery.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to related or dependent workstreams.").
			Security("non-sensitive").
			SystemUsage([]any{
				"dependency tracking",
				"navigation",
			}).
			Validation("must reference existing workstream object IDs.").
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
		WithProfileCode("WKS-014"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *WorkstreamBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *WorkstreamBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *WorkstreamBuilder) GetOntology() string {
	return "workstream"
}

func init() {
	builders.RegisterBuilder(NewWorkstreamBuilder())
}
