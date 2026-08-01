package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// WorkstreamTransitionBuilder builds the workstream_transition spec at version v2_0_0
// File: bldr_v2/workstream_transition_builder.go - version is encoded in package/directory name
type WorkstreamTransitionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewWorkstreamTransitionBuilder creates a new builder for workstream_transition spec version v2_0_0
func NewWorkstreamTransitionBuilder() *WorkstreamTransitionBuilder {
	builder := &WorkstreamTransitionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("workstream_transition", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines a transition from one workstream to another, including transition criteria, readiness checks, and agent onboarding requirements. Ensures smooth handoffs and strategic alignment in multi-year planning contexts.\\nLifecycle: workstream_transition_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addWorkstreamTransitionFields()

	return builder
}

// addWorkstreamTransitionFields adds the workstream_transition fields
func (b *WorkstreamTransitionBuilder) addWorkstreamTransitionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("agent_onboarding", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for agent readiness checks").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of agents that must be onboarded for this transition.").
			Security("non-sensitive").
			SystemUsage([]any{
				"agent_onboarding_management",
			}).
			Validation("List of agent onboarding objects").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("WST-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("from_workstream_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for transition validation").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("workstream must exist").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Reference to the workstream transitioning from.").
			Security("non-sensitive").
			SystemUsage([]any{
				"transition_tracking",
				"workflow_management",
			}).
			Validation("Must reference existing workstream").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("WST-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("readiness_criteria", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for readiness validation").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of criteria that must be met before transition can occur.").
			Security("non-sensitive").
			SystemUsage([]any{
				"transition_validation",
			}).
			Validation("List of criterion objects").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("WST-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("to_workstream_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for transition validation").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("workstream must exist").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Reference to the workstream transitioning to.").
			Security("non-sensitive").
			SystemUsage([]any{
				"transition_tracking",
				"workflow_management",
			}).
			Validation("Must reference existing workstream").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("WST-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("transition_date", "date").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for transition scheduling").
			Cardinality("one").
			Criticality("association").
			Default("optional").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Target date for this transition.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"scheduling",
			}).
			Validation("Valid date format").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "searchable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WST-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("trigger", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("determines transition logic").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("transition logic").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Type of trigger that initiates this transition.").
			Security("non-sensitive").
			SystemUsage([]any{
				"transition_automation",
				"workflow_management",
			}).
			Validation("enum").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"milestone_completion",
				"agent_readiness",
				"dependency_resolution",
				"strategic_pivot",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WST-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("trigger_milestone_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for milestone-based transitions").
			Cardinality("one").
			Criticality("association").
			Default("optional").
			Dependencies("milestone must exist").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Reference to milestone that triggers transition (if trigger is milestone_completion).").
			Security("non-sensitive").
			SystemUsage([]any{
				"transition_automation",
			}).
			Validation("Must reference existing milestone if provided").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "searchable", "writable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("WST-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *WorkstreamTransitionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *WorkstreamTransitionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *WorkstreamTransitionBuilder) GetOntology() string {
	return "workstream_transition"
}

func init() {
	builders.RegisterBuilder(NewWorkstreamTransitionBuilder())
}
