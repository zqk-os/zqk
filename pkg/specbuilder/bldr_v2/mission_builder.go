package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// MissionBuilder builds the mission spec at version v2_0_0
// File: bldr_v2/mission_builder.go - version is encoded in package/directory name
type MissionBuilder struct {
	*builders.BaseSpecBuilder
}

// NewMissionBuilder creates a new builder for mission spec version v2_0_0
func NewMissionBuilder() *MissionBuilder {
	builder := &MissionBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("mission", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Foundational statement describing the problem/mission/vision for a project or workstream.\\nLifecycle: mission_lifecycle.yaml.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addMissionFields()

	return builder
}

// addMissionFields adds the mission fields
func (b *MissionBuilder) addMissionFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for mission-goal alignment.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals that support this mission.").
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
		WithProfileCode("MIS-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("mission_statement", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive/owner.").
			AutomationHooks("seeds initial templates/prompts.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("docs, onboarding.").
			Lifecycle("mutable (with history).").
			Observability("yes").
			Purpose("Core statement of why this effort exists.").
			Security("may contain strategic info—treat as confidential by default.").
			SystemUsage([]any{
				"context",
				"prompting",
			}).
			Validation("markdown allowed; should cite personas/goals when possible.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("MIS-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("problem_statement", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive/owner.").
			AutomationHooks("used in requirement generation.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("requirements.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Defines the pain points motivating the mission.").
			Security("may contain strategic info.").
			SystemUsage([]any{
				"requirements",
				"scenario planning",
			}).
			Validation("markdown.").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("MIS-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("vision", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("executive.").
			AutomationHooks("can be referenced by templates for tone.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("goal alignment.").
			Lifecycle("mutable (tracked via change_log).").
			Observability("yes").
			Purpose("Desired future state / success narrative.").
			Security("confidential by default.").
			SystemUsage([]any{
				"strategy",
				"reports",
			}).
			Validation("markdown.").
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
		WithProfileCode("MIS-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("workstream_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for mission-workstream alignment.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("workstream registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Workstreams that support this mission.").
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
		WithProfileCode("MIS-005"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *MissionBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *MissionBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *MissionBuilder) GetOntology() string {
	return "mission"
}

func init() {
	builders.RegisterBuilder(NewMissionBuilder())
}
