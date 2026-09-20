package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// KindSynonymBuilder builds the kind_synonym spec at version v2_0_0
// File: bldr_v2/kind_synonym_builder.go - version is encoded in package/directory name
type KindSynonymBuilder struct {
	*builders.BaseSpecBuilder
}

// NewKindSynonymBuilder creates a new builder for kind_synonym spec version v2_0_0
func NewKindSynonymBuilder() *KindSynonymBuilder {
	builder := &KindSynonymBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("kind_synonym", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("null").
		SetDescription("Defines synonyms/aliases for object kinds to allow shorter, more convenient names in CLI commands. Synonyms must be unique per target_kind (no duplicate synonym entries for the same target_kind). If the same synonym is used for multiple target_kinds, the highest priority mapping wins.\\nLifecycle: kind_synonym_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addKindSynonymFields()

	return builder
}

// addKindSynonymFields adds the kind_synonym fields
func (b *KindSynonymBuilder) addKindSynonymFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("convention", "string").
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"two-word-first-letters",
				"single-word-prefix",
				"custom",
				"common-alias",
			}).
			Build()).
		WithTraits("filterable", "groupable").
		WithPermissions("r-x").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("description", "string").
		WithPermissions("rwx").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("id", "string").
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^SYN-\d{3,}$`).
			Build()).
		WithPermissions("rwx"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority", "integer").
		WithTraits("filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("synonym", "string").
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^[a-z0-9_-]+$`).
			Build()).
		WithTraits("filterable", "sortable", "unique").
		WithPermissions("r-x").
		WithSemanticType("statement"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_kind", "string").
		WithTraits("filterable", "sortable", "groupable").
		WithPermissions("r-x").
		WithSemanticType("reference"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *KindSynonymBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *KindSynonymBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *KindSynonymBuilder) GetOntology() string {
	return "kind_synonym"
}

func init() {
	builders.RegisterBuilder(NewKindSynonymBuilder())
}
