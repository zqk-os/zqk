package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// GoalBuilder builds the goal spec at version v2_0_0
// File: bldr_v2/goal_builder.go - version is encoded in package/directory name
type GoalBuilder struct {
	*builders.BaseSpecBuilder
}

// NewGoalBuilder creates a new builder for goal spec version v2_0_0
func NewGoalBuilder() *GoalBuilder {
	builder := &GoalBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("goal", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("work_interval").
		SetDescription("Goal objects define measurable outcomes linked to metrics, authority, and workstreams.\\nLifecycle: goal_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable").
		AddTrait("completable").
		AddTrait("status_reactive")

	// Add fields
	builder.addGoalFields()

	return builder
}

// addGoalFields adds the goal fields
func (b *GoalBuilder) addGoalFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("authority", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive.").
			AutomationHooks("used for notifications and routing.").
			Cardinality("one").
			Criticality("composition").
			Default("required at creation").
			Dependencies("role registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Role/person responsible for goal achievement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
				"notifications",
			}).
			Validation("must match role registry.").
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
		WithProfileCode("GOL-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metric", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("display only.").
			Cardinality("one").
			Criticality("association").
			Default("auto-populated from template").
			Dependencies("documentation.").
			Lifecycle("mutable (but should stay aligned with template).").
			Observability("yes").
			Purpose("Human-readable metric description (may mirror template).").
			Security("non-sensitive").
			SystemUsage([]any{
				"docs",
				"communication",
			}).
			Validation("<= 120 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(120).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("expression").
		WithProfileCode("GOL-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metric_template_id", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive/owner with metric permission.").
			AutomationHooks("metrics evaluation pipeline.").
			Cardinality("one").
			Criticality("composition").
			Default("none (required at creation)").
			Dependencies("metrics engine.").
			Lifecycle("immutable (can change via decision but treat carefully).").
			Observability("yes").
			Purpose("Reference to metric template governing measurement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"automation",
			}).
			Validation("must exist in \\\\\\\"METRIC_TEMPLATES.yaml\\\\\\\".").
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
		WithProfileCode("GOL-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used in evaluation logic.").
			Cardinality("one").
			Criticality("composition").
			Default("required at creation").
			Dependencies("metrics engine.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Success threshold for the metric.").
			Security("non-sensitive").
			SystemUsage([]any{
				"evaluation",
				"reports",
			}).
			Validation("format depends on metric type.").
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
		WithProfileCode("GOL-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("used for goal progress calculation.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams that contribute to achieving this goal.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"reporting",
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
		WithProfileCode("GOL-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("epic_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/planner.").
			AutomationHooks("maps strategic goal into concrete enclave epics.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("epic registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Epics directly executing towards achieving this strategic goal.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"traceability",
				"alignment",
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
		WithProfileCode("GOL-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("success_criteria", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/executive.").
			AutomationHooks("evaluation of goal achievement conditions.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Explicit conditions, invariants, or criteria that must be satisfied for goal achievement.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"evaluation",
				"reporting",
			}).
			Validation("list of criteria statement strings.").
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
		WithProfileCode("GOL-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("vision_ref", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive.").
			AutomationHooks("aligns strategic goals with parent vision.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("vision registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Vision object governing this strategic goal.").
			Security("non-sensitive").
			SystemUsage([]any{
				"alignment",
				"traceability",
			}).
			Validation("must reference an existing vision ID.").
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
		WithProfileCode("GOL-014"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *GoalBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *GoalBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *GoalBuilder) GetOntology() string {
	return "goal"
}

func init() {
	builders.RegisterBuilder(NewGoalBuilder())
}
