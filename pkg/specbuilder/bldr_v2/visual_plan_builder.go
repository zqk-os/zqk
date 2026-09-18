package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// VisualPlanBuilder builds the visual_plan spec at version v2_0_0
// File: bldr_v2/visual_plan_builder.go - version is encoded in package/directory name
type VisualPlanBuilder struct {
	*builders.BaseSpecBuilder
}

// NewVisualPlanBuilder creates a new builder for visual_plan spec version v2_0_0
func NewVisualPlanBuilder() *VisualPlanBuilder {
	builder := &VisualPlanBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("visual_plan", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Models a visual plan including marketing vision, context, scenes, and shots.").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable")

	// Add fields
	builder.addVisualPlanFields()

	return builder
}

// addVisualPlanFields adds the visual_plan fields
func (b *VisualPlanBuilder) addVisualPlanFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("marketing_vision", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creative director.").
			AutomationHooks("none.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("The marketing vision.").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
			}).
			Validation("string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VPL-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("project_context", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("project manager.").
			AutomationHooks("none.").
			Cardinality("one").
			Criticality("core").
			Default("").
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Project context.").
			Security("non-sensitive").
			SystemUsage([]any{
				"context",
			}).
			Validation("string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VPL-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scenes", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creative director.").
			AutomationHooks("none.").
			Cardinality("many").
			Criticality("core").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Scenes in the plan.").
			Security("non-sensitive").
			SystemUsage([]any{
				"breakdown",
			}).
			Validation("list of strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VPL-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("shots", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("creative director.").
			AutomationHooks("none.").
			Cardinality("many").
			Criticality("core").
			Default([]any{}).
			Dependencies("scenes.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Shots in the plan.").
			Security("non-sensitive").
			SystemUsage([]any{
				"breakdown",
			}).
			Validation("list of strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VPL-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *VisualPlanBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *VisualPlanBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *VisualPlanBuilder) GetOntology() string {
	return "visual_plan"
}

func init() {
	builders.RegisterBuilder(NewVisualPlanBuilder())
}
