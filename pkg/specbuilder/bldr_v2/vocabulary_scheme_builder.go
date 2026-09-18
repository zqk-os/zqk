package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// VocabularySchemeBuilder builds the vocabulary_scheme spec at version v2_0_0
// File: bldr_v2/vocabulary_scheme_builder.go - version is encoded in package/directory name
type VocabularySchemeBuilder struct {
	*builders.BaseSpecBuilder
}

// NewVocabularySchemeBuilder creates a new builder for vocabulary_scheme spec version v2_0_0
func NewVocabularySchemeBuilder() *VocabularySchemeBuilder {
	builder := &VocabularySchemeBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("vocabulary_scheme", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Names a vocabulary graph (taxonomy / lens network / navigation scheme) so typed edges\\n(glossary_term_relation) can be scoped. Multiple schemes can coexist over the same glossary\\nterms (e.g. inference predicates vs display groupings). purpose distinguishes machine\\nbehavior surfaces from presentation-only groupings. Future SKOS/SHACL-style imports can map\\nto schemes with purpose=extension. Lifecycle: vocabulary_scheme_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addVocabularySchemeFields()

	return builder
}

// addVocabularySchemeFields adds the vocabulary_scheme fields
func (b *VocabularySchemeBuilder) addVocabularySchemeFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("allowed_kinds", "list").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("context_scope", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithDefault("").
		WithTraits("filterable", "groupable").
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
	b.AddFieldBuilder(builders.NewFieldBuilder("purpose", "string").
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"inference",
				"display",
				"navigation",
				"mixed",
				"extension",
			}).
			Required(true).
			Build()).
		WithDefault("mixed").
		WithTraits("filterable", "groupable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("summary", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "searchable").
		WithPermissions("rwx").
		WithSemanticType("statement"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *VocabularySchemeBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *VocabularySchemeBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *VocabularySchemeBuilder) GetOntology() string {
	return "vocabulary_scheme"
}

func init() {
	builders.RegisterBuilder(NewVocabularySchemeBuilder())
}
