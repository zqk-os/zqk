package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// StrategicPlanBuilder builds the strategic_plan spec at version v2_0_0
// File: bldr_v2/strategic_plan_builder.go - version is encoded in package/directory name
type StrategicPlanBuilder struct {
	*builders.BaseSpecBuilder
}

// NewStrategicPlanBuilder creates a new builder for strategic_plan spec version v2_0_0
func NewStrategicPlanBuilder() *StrategicPlanBuilder {
	builder := &StrategicPlanBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("strategic_plan", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines multi-year strategic plans with phases, workstreams, goals, and agent onboarding timelines. Enables long-term planning and strategic alignment across multiple years.\\nLifecycle: strategic_plan_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addStrategicPlanFields()

	return builder
}

// addStrategicPlanFields adds the strategic_plan fields
func (b *StrategicPlanBuilder) addStrategicPlanFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for goal tracking").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goals must exist").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to all goals in this strategic plan.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"reporting",
			}).
			Validation("Must reference existing goals").
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
		WithProfileCode("STRAT-004"))
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
			Purpose("Stable identifier used across the graph (\\\\\\\"STRAT-PLAN-####\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"linking",
				"reporting",
				"URIs",
			}).
			Validation("regex ^STRAT-PLAN-\\\\\\\\d{3,}$; uniqueness enforced.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^STRAT-PLAN-\d{3,}$`).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("phases", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive/admin").
			AutomationHooks("used for phase management").
			Cardinality("many").
			Criticality("composition").
			Default("required").
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("List of phases in this strategic plan, each with workstreams, goals, and agent onboarding.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"reporting",
				"workflow_management",
			}).
			Validation("List of phase objects").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("STRAT-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("planning_horizon", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive/admin").
			AutomationHooks("used for plan validation").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("none").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Time period covered by this strategic plan (e.g., \\\\\\\"2026-01-01 to 2028-12-31\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"reporting",
			}).
			Validation("Date range format").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("STRAT-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/admin").
			AutomationHooks("used for workstream tracking").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstreams must exist").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("References to all workstreams in this strategic plan.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"reporting",
			}).
			Validation("Must reference existing workstreams").
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
		WithProfileCode("STRAT-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *StrategicPlanBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *StrategicPlanBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *StrategicPlanBuilder) GetOntology() string {
	return "strategic_plan"
}

func init() {
	builders.RegisterBuilder(NewStrategicPlanBuilder())
}
