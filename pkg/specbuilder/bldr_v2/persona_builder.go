package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// PersonaBuilder builds the persona spec at version v2_0_0
// File: bldr_v2/persona_builder.go - version is encoded in package/directory name
type PersonaBuilder struct {
	*builders.BaseSpecBuilder
}

// NewPersonaBuilder creates a new builder for persona spec version v2_0_0
func NewPersonaBuilder() *PersonaBuilder {
	builder := &PersonaBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("persona", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Personas describe representative users/stakeholders referenced by missions, requirements, and templates.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addPersonaFields()

	return builder
}

// addPersonaFields adds the persona fields
func (b *PersonaBuilder) addPersonaFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("agent_skill_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("CRI-PERSONA-SKILL-BOUND readiness / CAP dispatch gate.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("agent_skill registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Canonical agent_skill (ASK-*) links required for persona dispatch readiness.").
			Security("non-sensitive").
			SystemUsage([]any{
				"dispatch",
				"seating",
				"prompts",
			}).
			Validation("Must reference existing agent_skill IDs.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("PER-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("interaction_policy_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("POL-AGENT-INTERACTION-POLICY-001 evaluator (guiding-step ping-pong).").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("policy registry (POL-AGENT-* interaction policies).").
			Lifecycle("mutable.").
			Observability("yes.").
			Purpose("Persona-bound interaction policies. Dual-read with related_object_refs until all personas are migrated.").
			Security("non-sensitive.").
			SystemUsage([]any{
				"dispatch",
				"seating",
				"prompts",
			}).
			Validation("Must reference existing policy IDs (POL-*).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("PER-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goal_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for persona-goal alignment.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("goal registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Goals this persona supports.").
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
		WithProfileCode("PER-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("mission_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used for persona-mission alignment.").
			Cardinality("many").
			Criticality("association").
			Default([]any{}).
			Dependencies("mission registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Missions this persona supports.").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"alignment",
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
		WithProfileCode("PER-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("needs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used in requirement generation.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("requirements.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Bullet list of needs/pain points.").
			Security("non-sensitive").
			SystemUsage([]any{
				"requirements",
				"scenario design",
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
		WithSemanticType("statement").
		WithProfileCode("PER-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("role", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("none.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("scenario planning.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Persona's job/context.").
			Security("may contain sensitive org info—mark accordingly.").
			SystemUsage([]any{
				"prompting",
			}).
			Validation("3–80 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(80).
			MinLength(3).
			Pattern(`^.+$`).
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("PER-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("vocabulary_scheme_refs", "list").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("Used by CLI to apply semantic filtering.").
			Cardinality("many").
			Criticality("composition").
			Default([]any{}).
			Dependencies("vocabulary_scheme.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Semantic vocabularies bound to this persona.").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"semantic resolution",
			}).
			Validation("list of strings.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group", "field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("PER-006"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *PersonaBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *PersonaBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *PersonaBuilder) GetOntology() string {
	return "persona"
}

func init() {
	builders.RegisterBuilder(NewPersonaBuilder())
}
