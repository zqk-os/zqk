package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// BacklogItemBuilder builds the backlog_item spec at version v2_0_0
// File: bldr_v2/backlog_item_builder.go - version is encoded in package/directory name
type BacklogItemBuilder struct {
	*builders.BaseSpecBuilder
}

// NewBacklogItemBuilder creates a new builder for backlog_item spec version v2_0_0
func NewBacklogItemBuilder() *BacklogItemBuilder {
	builder := &BacklogItemBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("backlog_item", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Backlog items represent feature ideas, enhancements, and work items that are being explored, validated, or planned. They progress through a lifecycle from exploration to completion, and can be linked to goals, milestones, requirements, and documents for traceability.\\nWork that is planned or in progress must be traceable through milestones (and priority plans) to mission, vision, goals, and strategic plan; lifecycle preconditions enforce milestone linkage before planned and in_progress can be held without error.\\nLifecycle: backlog_item_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable").
		AddTrait("constrainable")

	// Add fields
	builder.addBacklogItemFields()

	return builder
}

// addBacklogItemFields adds the backlog_item fields
func (b *BacklogItemBuilder) addBacklogItemFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("acceptance_criteria", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for completion validation, test case generation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("criteria registry (for references).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Verifiable conditions that must be met for the backlog item to be considered complete. Can contain either free-form text strings or criteria object references (CRIT-####) for traceability.").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
				"testing",
				"completion tracking",
				"traceability",
			}).
			Validation("List items can be either strings (free-form text) or criteria object references (CRIT-#### format). References must point to existing criteria objects for traceability.").
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
		WithProfileCode("ITEM-016"))
	b.AddFieldBuilder(builders.NewFieldBuilder("benefits", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in prioritization, justification reports.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of benefits this feature provides (user value, technical value, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"justification",
				"prioritization",
				"communication",
			}).
			Validation("list of strings.").
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
		WithProfileCode("ITEM-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("category", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/product.").
			AutomationHooks("used for filtering, reporting, prioritization.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-extracted from description if config available").
			Dependencies("category extraction rules.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Category classification (e.g., \\\\\\\"Developer Experience\\\\\\\", \\\\\\\"Product\\\\\\\", \\\\\\\"Reliability\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"prioritization",
			}).
			Validation("Should match controlled vocabulary (see category extraction config).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ITEM-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("commit_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for linking commits to backlog items.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("Version control repository (Git, SVN, Mercurial, Perforce, etc.).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Version control system commit identifiers that implement or relate to this backlog item.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"code-to-project graph",
			}).
			Validation("Must be valid VCS commit identifiers. Format depends on VCS system:\n- Git: SHA-1 hash (40 chars) or short hash (7+ chars)\n- SVN: Revision number (numeric)\n- Mercurial: Hex hash (40 chars)\n- Perforce: Changelist number (numeric)\n- Other: Alphanumeric string, 1-40 characters\n").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[A-Za-z0-9_-]{1,40}$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ITEM-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("completed_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner.").
			AutomationHooks("automatically set when status changes to complete.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("lifecycle transitions.").
			Lifecycle("mutable (set when status transitions to complete).").
			Observability("yes").
			Purpose("ISO-8601 datetime when the backlog item was completed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"metrics",
				"velocity calculation",
			}).
			Validation("ISO-8601 datetime format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ITEM-017"))
	b.AddFieldBuilder(builders.NewFieldBuilder("components", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for work breakdown, milestone creation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of components or sub-features that make up this item.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"breakdown",
				"tracking",
			}).
			Validation("list of strings.").
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
		WithProfileCode("ITEM-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("considerations", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in planning, risk reports.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Implementation considerations, risks, or constraints.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"risk assessment",
				"decision making",
			}).
			Validation("list of strings.").
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
		WithProfileCode("ITEM-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("context", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in reports, justification documents.").
			Cardinality("one").
			Criticality("association").
			Default("none (optional)").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("When/why the idea surfaced (meeting, user feedback, technical debt, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"context",
				"justification",
				"traceability",
			}).
			Validation("free text.").
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
		WithProfileCode("ITEM-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("convergence_session_profile", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("workflow next output, dashboards.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Short label for how the linked CVS is used (e.g. vetting_matrix, health, nested_orchestration).").
			Security("non-sensitive").
			SystemUsage([]any{
				"display",
				"filtering",
			}).
			Validation("Free text when convergence_session_ref is set.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("ITEM-020"))
	b.AddFieldBuilder(builders.NewFieldBuilder("convergence_session_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("workflow next, convergence reporting, vetting matrix alignment.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("").
			Dependencies("convergence_session registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional link to the convergence session that owns or measures work for this backlog item.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"workflow routing",
				"nested CVS coordination",
			}).
			Validation("Must reference an existing convergence_session id when set.").
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
		WithProfileCode("ITEM-019"))
	b.AddFieldBuilder(builders.NewFieldBuilder("date_captured", "date").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for date-based filtering, backlog age reports.").
			Cardinality("one").
			Criticality("association").
			Default("auto-populated from created_at if not provided").
			Dependencies("creation timestamp.").
			Lifecycle("immutable (set at creation).").
			Observability("yes").
			Purpose("ISO-8601 date when the idea was first captured.").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"reporting",
				"age calculation",
			}).
			Validation("ISO-8601 date format (YYYY-MM-DD).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}$`).
			Required(false).
			Build()).
		WithTraits("filterable", "listable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("ITEM-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in markdown generation, reports.").
			Cardinality("one").
			Criticality("composition").
			Default("none (optional but recommended)").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Detailed description of the feature or enhancement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"planning",
				"communication",
			}).
			Validation("free text, recommended >= 50 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MinLength(0).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ITEM-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("document_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for dependency tree construction, document discovery.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("doc index, resolver.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to documents (paths or doc IDs) related to this backlog item.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"documentation discovery",
				"dependency graphs",
			}).
			Validation("Document paths or doc IDs from doc_index.json.").
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
		WithProfileCode("ITEM-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used to trace goal support, generate goal reports.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to goals this backlog item supports.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"justification",
				"goal coverage",
			}).
			Validation("Must reference existing goal IDs.").
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
		WithProfileCode("ITEM-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("milestone_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for percent-complete calculation, auto-transition to complete.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("milestone registry, lifecycle (auto-complete when all complete).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to milestones that implement this backlog item. Milestones are the primary link from\nexecution back to strategic context (mission, vision, goals, strategic plan); planned and\nin_progress statuses require at least one milestone reference so work stays traceable.\n").
			Security("non-sensitive").
			SystemUsage([]any{
				"progress tracking",
				"lifecycle transitions",
				"completion",
			}).
			Validation("Must reference existing milestone IDs.").
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
		WithProfileCode("ITEM-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("model_tier", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("Used by the orchestrator to route task execution difficulty.").
			Cardinality("one").
			Criticality("metadata").
			Default("tier_2_simple").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The model tier required to execute tasks generated from this backlog item (e.g. tier_1_complex, tier_2_simple).").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
			}).
			Validation("Must be a string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "modifiable", "readable", "searchable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ITEM-022"))
	b.AddFieldBuilder(builders.NewFieldBuilder("notes", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in reports, markdown generation.").
			Cardinality("one").
			Criticality("association").
			Default("none (optional)").
			Dependencies("none.").
			Lifecycle("mutable (append-only recommended).").
			Observability("yes").
			Purpose("Additional notes, decisions, or context about this backlog item.").
			Security("non-sensitive").
			SystemUsage([]any{
				"documentation",
				"context",
				"history",
			}).
			Validation("free text.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("ITEM-015"))
	b.AddFieldBuilder(builders.NewFieldBuilder("persona_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for persona-based filtering.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("persona registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Personas assigned or relevant to this backlog item.").
			Security("non-sensitive").
			SystemUsage([]any{
				"grouping",
				"reporting",
				"routing",
			}).
			Validation("must reference existing persona IDs.").
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
		WithProfileCode("ITEM-021"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("product/owner.").
			AutomationHooks("used for roadmap ordering, priority-based filtering.").
			Cardinality("one").
			Criticality("composition").
			Default("none (optional until roadmap commitment)").
			Dependencies("roadmap planning.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Priority level (critical, high, medium, low) aligned with priority tiers (P0=critical, P1=high, P2=medium, P3=low).").
			Security("non-sensitive").
			SystemUsage([]any{
				"prioritization",
				"roadmap ordering",
				"filtering",
			}).
			Validation("enum (critical, high, medium, low).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"critical",
				"high",
				"medium",
				"low",
			}).
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ITEM-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority_plan_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/product.").
			AutomationHooks("used for plan-based filtering, workstream derivation, future planning.").
			Cardinality("one").
			Criticality("composition").
			Default("none (optional for exploring/validated, required for planned/in_progress/complete)").
			Dependencies("priority_plan registry.").
			Lifecycle("mutable (via explicit move command).").
			Observability("yes").
			Purpose("Reference to the priority plan this backlog item belongs to (1:1 relationship, enforced). Required for items beyond exploring/validated status per DEC-priority-plan-ref-requirement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"grouping",
				"reporting",
				"future planning",
			}).
			Validation("Must reference existing priority_plan object ID. Required for status beyond exploring/validated.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^(PLAN-[\w-]+|PRIO-[\w-]+|priority-plan-[\w-]+)$`).
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("ITEM-035"))
	b.AddFieldBuilder(builders.NewFieldBuilder("related_features", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for dependency graphs, related item discovery.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("backlog registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to related backlog items or features (by ID or title).").
			Security("non-sensitive").
			SystemUsage([]any{
				"discovery",
				"grouping",
				"dependency tracking",
			}).
			Validation("list of strings (IDs or titles).").
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
		WithProfileCode("ITEM-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("requirement_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used to verify requirement coverage, generate traceability reports.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("requirement registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to requirements this backlog item fulfills.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"requirement coverage",
			}).
			Validation("Must reference existing requirement IDs.").
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
		WithProfileCode("ITEM-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("lifecycle rules (see backlog_item_lifecycle.yaml).").
			AutomationHooks("lifecycle transitions, status validation.").
			Cardinality("one").
			Criticality("composition").
			Default("exploring").
			Dependencies("lifecycle registry.").
			Lifecycle("mutable (via lifecycle transitions).").
			Observability("yes").
			Purpose("Lifecycle status (exploring, validated, planned, in_progress, complete, archived, rejected). Note: 'workstream' status replaced with 'planned' per DEC-backlog-item-priority-plan-1-1-relationship. Statuses planned and in_progress require priority_plan_ref and milestone linkage so items cannot be worked under those statuses until traceability to the strategic hierarchy is in place.").
			Security("non-sensitive").
			SystemUsage([]any{
				"lifecycle enforcement",
				"filtering",
				"reporting",
			}).
			Validation("Must match lifecycle definition.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"exploring",
				"validated",
				"roadmap",
				"deferred",
				"planned",
				"in_progress",
				"complete",
				"archived",
				"rejected",
				"error",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("ITEM-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for workstream progress calculation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams this backlog item belongs to or contributes to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"grouping",
				"reporting",
				"planning",
			}).
			Validation("must reference existing workstream IDs.").
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
		WithProfileCode("ITEM-018"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *BacklogItemBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *BacklogItemBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *BacklogItemBuilder) GetOntology() string {
	return "backlog_item"
}

func init() {
	builders.RegisterBuilder(NewBacklogItemBuilder())
}
