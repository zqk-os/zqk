package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
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
		AddCompose("remaining_open").
		SetDescription("Represents a stage/tier/step within or across workstreams. Smaller than a full workstream but larger than a single requirement, used to track prerequisites/blockers and progress.\\nLifecycle: milestone_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable").
		AddTrait("effort_aware").
		AddTrait("status_reactive").
		AddTrait("open_countable").
		AddTrait("summarizes_children")

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
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("MLS-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("criteria_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for milestone verification and completion validation.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("criteria registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Canonical typed links to criteria objects (CRIT-*) that must be satisfied for this milestone.").
			Security("non-sensitive").
			SystemUsage([]any{"validation", "completion tracking", "traceability"}).
			Validation("must reference existing criteria object IDs (CRIT-* format).").
			Build()).
		WithAccess(builders.NewAccessBuilder().Requires("access:confidential").Build()).
		WithValidation(builders.NewValidationBuilder().Required(false).Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("MLS-011"))
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
		WithTraits("field_mutable_group", "field_reference_group").
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
		WithTraits("field_mutable_group", "field_reference_group").
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
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("MLS-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("epic_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/planner.").
			AutomationHooks("associates milestone checkpoints with governing epics.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("epic registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Epics this milestone tracks or contributes to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"reporting",
			}).
			Validation("must reference existing epic IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("MLS-012"))
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
