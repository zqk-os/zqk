package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// GlossaryTermRelationBuilder builds the glossary_term_relation spec at version v2_0_0
// File: bldr_v2/glossary_term_relation_builder.go - version is encoded in package/directory name
type GlossaryTermRelationBuilder struct {
	*builders.BaseSpecBuilder
}

// NewGlossaryTermRelationBuilder creates a new builder for glossary_term_relation spec version v2_0_0
func NewGlossaryTermRelationBuilder() *GlossaryTermRelationBuilder {
	builder := &GlossaryTermRelationBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("glossary_term_relation", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Directed, typed link between two glossary_term rows, scoped to a vocabulary_scheme.\\npredicate_ref points to a glossary_term whose category documents the predicate semantics\\n(e.g. broader, narrower, related, instantiates). source_term_ref and target_term_ref are\\nglossary_term ids (GLS-*). scheme_ref is vocabulary_scheme (VOC-*). Enables multiple\\nparallel taxonomies over the same terms. Lifecycle: glossary_term_relation_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addGlossaryTermRelationFields()

	return builder
}

// addGlossaryTermRelationFields adds the glossary_term_relation fields
func (b *GlossaryTermRelationBuilder) addGlossaryTermRelationFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("notes", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("predicate_ref", "string").
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^GLS-`).
			Required(true).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("scheme_ref", "string").
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^VOC-`).
			Required(true).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("sort_order", "integer").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source_term_ref", "string").
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^GLS-`).
			Required(true).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_term_ref", "string").
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^GLS-`).
			Required(true).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("rwx").
		WithSemanticType("reference"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *GlossaryTermRelationBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *GlossaryTermRelationBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *GlossaryTermRelationBuilder) GetOntology() string {
	return "glossary_term_relation"
}

func init() {
	builders.RegisterBuilder(NewGlossaryTermRelationBuilder())
}
