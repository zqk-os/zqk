package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// MilestoneBuilder builds the milestone spec at version v2_0_0
// File: bldr_v2/milestone_builder.go - version is encoded in package/directory name
type MilestoneBuilder struct {
	*builders.BaseSpecBuilder
}

// NewMilestoneBuilder creates a new builder for milestone spec version v2_0_0
func NewMilestoneBuilder() *MilestoneBuilder {
	builder := &MilestoneBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("milestone", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("work_unit").
		SetDescription("Represents a stage/tier/step within or across workstreams. Smaller than a full workstream but larger than a single requirement, used to track prerequisites/blockers and progress.\\nLifecycle: milestone_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable").
		AddTrait("effort_aware").
		AddTrait("status_reactive")

	// Add fields
	builder.addMilestoneFields()

	return builder
}

// addMilestoneFields adds the milestone fields
func (b *MilestoneBuilder) addMilestoneFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("blocked_by_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for blocking analysis and reports.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("object registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones or work items blocking this milestone.").
			Security("non-sensitive").
			SystemUsage([]any{
				"blocking checks",
				"reporting",
			}).
			Validation("must reference existing object IDs.").
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
		WithProfileCode("MLS-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("commit_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for linking commits to milestones.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("Version control repository (Git, SVN, Mercurial, Perforce, etc.).").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Version control system commit identifiers that implement or relate to this milestone.").
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
		WithTraits("filterable", "modifiable", "readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("MLS-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("completion_criteria", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for completion validation.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Verifiable conditions that must be met for milestone completion.").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
				"completion tracking",
			}).
			Validation("list of strings describing criteria.").
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
		WithProfileCode("MLS-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("criteria_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for milestone completion validation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("criteria registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Criteria this milestone must satisfy for completion.").
			Security("non-sensitive").
			SystemUsage([]any{
				"validation",
				"completion tracking",
			}).
			Validation("must reference existing criteria IDs.").
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
		WithProfileCode("MLS-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for goal progress calculation.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals this milestone contributes to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"reporting",
			}).
			Validation("must reference existing goal IDs.").
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
		WithProfileCode("MLS-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("prerequisite_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("prevents milestone from starting until prerequisites complete.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("milestone registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestones that must be completed before this one can start.").
			Security("non-sensitive").
			SystemUsage([]any{
				"blocking checks",
				"planning",
			}).
			Validation("must reference existing milestone IDs.").
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
		WithProfileCode("MLS-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("stage_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used to order milestones.").
			Cardinality("one").
			Criticality("composition").
			Default("stage").
			Dependencies("dashboards.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Classify the milestone (tier, stage, prerequisite, validation, release gate, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
			}).
			Validation("enum; extendable via config.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"tier",
				"stage",
				"prerequisite",
				"validation",
				"release_gate",
			}).
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "modifiable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("MLS-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("prevents dependent workstreams from starting when blocked.").
			Cardinality("one").
			Criticality("composition").
			Default("not_started").
			Dependencies("prerequisite logic.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Milestone status (\\\\\\\"not_started\\\\\\\", \\\\\\\"in_progress\\\\\\\", \\\\\\\"blocked\\\\\\\", \\\\\\\"complete\\\\\\\", \\\\\\\"deferred\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"blocking checks",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"not_started",
				"in_progress",
				"blocked",
				"complete",
				"deferred",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "modifiable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("MLS-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status_history", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/lifecycle system.").
			AutomationHooks("automatically populated on status transitions.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("lifecycle system.").
			Lifecycle("append-only.").
			Observability("yes").
			Purpose("Chronological record of status changes and notes.").
			Security("non-sensitive").
			SystemUsage([]any{
				"audits",
				"reports",
			}).
			Validation("List of StatusHistoryEntry objects.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("MLS-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for workstream progress calculation.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams this milestone belongs to or impacts.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"grouping",
			}).
			Validation("must reference existing workstream IDs.").
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
		WithProfileCode("MLS-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *MilestoneBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *MilestoneBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *MilestoneBuilder) GetOntology() string {
	return "milestone"
}

func init() {
	builders.RegisterBuilder(NewMilestoneBuilder())
}
