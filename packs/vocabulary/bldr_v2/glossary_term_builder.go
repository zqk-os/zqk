package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// GlossaryTermBuilder builds the glossary_term spec at version v2_0_0
// File: bldr_v2/glossary_term_builder.go - version is encoded in package/directory name
type GlossaryTermBuilder struct {
	*builders.BaseSpecBuilder
}

// NewGlossaryTermBuilder creates a new builder for glossary_term spec version v2_0_0
func NewGlossaryTermBuilder() *GlossaryTermBuilder {
	builder := &GlossaryTermBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("glossary_term", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Operational glossary term: a context-scoped definition for agents and tooling.\\nUse for shared vocabulary (e.g. \\\"new system object\\\", \\\"process data\\\") so agents\\nand automation use consistent definitions. Link to existing alias/synonym objects\\nvia alias_refs. context_scope, category, agent_prompts, and machine_hints are\\nrequired; defaults are auto-resolved at create if omitted.\\nsemantic_tags classifies usage: e.g. ontology, epistemology, inference, display,\\nnavigation, predicate_definition (for terms that define glossary_term_relation predicates).\\nLifecycle: glossary_term_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addGlossaryTermFields()

	return builder
}

// addGlossaryTermFields adds the glossary_term fields
func (b *GlossaryTermBuilder) addGlossaryTermFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("agent_prompts", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithDefault("").
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("alias_refs", "list").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("category", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithDefault("").
		WithTraits("filterable", "listable", "groupable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("context_scope", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithDefault("").
		WithTraits("filterable", "groupable").
		WithPermissions("r-x").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("definition", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("machine_hints", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithDefault("").
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scheme_ref", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Pattern(`^VOC-`).
			Build()).
		WithDefault("").
		WithTraits("field_reference_group", "filterable", "groupable").
		WithPermissions("rwx").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("semantic_tags", "list").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *GlossaryTermBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *GlossaryTermBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *GlossaryTermBuilder) GetOntology() string {
	return "glossary_term"
}

func init() {
	builders.RegisterBuilder(NewGlossaryTermBuilder())
}
