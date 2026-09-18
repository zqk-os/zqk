package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// TranslationManifestBuilder builds the translation_manifest spec at version v1_0_0
type TranslationManifestBuilder struct {
	*builders.BaseSpecBuilder
}

// NewTranslationManifestBuilder creates a new builder for translation_manifest spec version v1_0_0
func NewTranslationManifestBuilder() *TranslationManifestBuilder {
	builder := &TranslationManifestBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("translation_manifest", "v1_0_0"),
	}

	builder.
		SetExtends("base_object").
		SetDescription("Manifest for mapping external schema formats to ZQK internal ontology.").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits").
		AddTrait("manipulatable").
		AddTrait("constrainable")

	builder.addFields()
	return builder
}

func (b *TranslationManifestBuilder) addFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("source_url", "string").
		WithChecklist(builders.NewChecklistBuilder().Purpose("URI of the external schema.").Cardinality("one").Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("translator_id", "string").
		WithChecklist(builders.NewChecklistBuilder().Purpose("Identifier for the registered SemanticTranslator.").Cardinality("one").Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_kind", "string").
		WithChecklist(builders.NewChecklistBuilder().Purpose("The ZQK object kind produced by this translation.").Cardinality("one").Build()))
}

func (b *TranslationManifestBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

func (b *TranslationManifestBuilder) GetVersion() string {
	return "v1_0_0"
}

func (b *TranslationManifestBuilder) GetOntology() string {
	return "translation_manifest"
}

func init() {
	builders.RegisterBuilder(NewTranslationManifestBuilder())
}
