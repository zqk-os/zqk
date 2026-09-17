package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// VisionBuilder builds the vision spec at version v2_0_0
// File: bldr_v2/vision_builder.go - version is encoded in package/directory name
type VisionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewVisionBuilder creates a new builder for vision spec version v2_0_0
func NewVisionBuilder() *VisionBuilder {
	builder := &VisionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("vision", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Long-term vision statement describing the desired future state and success narrative for a project, workstream, or organization. Provides strategic direction and alignment.\\nLifecycle: vision_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addVisionFields()

	return builder
}

// addVisionFields adds the vision fields
func (b *VisionBuilder) addVisionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for vision-goal alignment.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals that support this vision.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"alignment",
			}).
			Validation("Must reference existing goal IDs.").
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
		WithProfileCode("VIS-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("mission_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for vision-mission alignment.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("mission registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Mission statements this vision aligns with.").
			Security("non-sensitive").
			SystemUsage([]any{
				"navigation",
				"reporting",
			}).
			Validation("Must reference existing mission IDs.").
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
		WithProfileCode("VIS-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("narrative", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive/owner.").
			AutomationHooks("can be referenced by templates for tone and direction.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("mission, goals.").
			Lifecycle("mutable (with history).").
			Observability("yes").
			Purpose("Core vision narrative describing the desired future state.").
			Security("may contain strategic info—treat as confidential by default.").
			SystemUsage([]any{
				"strategy",
				"communication",
				"prompting",
			}).
			Validation("markdown allowed; should align with mission and goals.").
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
		WithProfileCode("VIS-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("pillars", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive/owner.").
			AutomationHooks("can be used for alignment checks.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("none.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Key pillars or principles that support the vision (e.g., \\\\\\\"Trustworthy automation\\\\\\\", \\\\\\\"Composable drivers\\\\\\\").").
			Security("may contain strategic information.").
			SystemUsage([]any{
				"strategy",
				"communication",
			}).
			Validation("list of strings describing pillars.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("VIS-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for vision-workstream alignment.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams that support this vision.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"alignment",
			}).
			Validation("Must reference existing workstream IDs.").
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
		WithProfileCode("VIS-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *VisionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *VisionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *VisionBuilder) GetOntology() string {
	return "vision"
}

func init() {
	builders.RegisterBuilder(NewVisionBuilder())
}
