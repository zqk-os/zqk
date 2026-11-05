package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// IoCacheBuilder builds the io_cache spec at version v2_0_0
type IoCacheBuilder struct {
	*builders.BaseSpecBuilder
}

// NewIoCacheBuilder creates a new builder for io_cache spec version v2_0_0
func NewIoCacheBuilder() *IoCacheBuilder {
	builder := &IoCacheBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("io_cache", "v2_0_0"),
	}

	builder.
		SetExtends("base_object").
		SetDescription("Universal External Respiration Gateway and CAS Scraping Cache").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	builder.addIoCacheFields()

	return builder
}

func (b *IoCacheBuilder) addIoCacheFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("url", "string").
		WithTraits("readable", "writable").
		WithSemanticType("location"))

	b.AddFieldBuilder(builders.NewFieldBuilder("domain", "string").
		WithTraits("readable", "writable").
		WithSemanticType("location"))

	b.AddFieldBuilder(builders.NewFieldBuilder("raw_hash", "string").
		WithTraits("readable", "writable").
		WithSemanticType("digest"))

	b.AddFieldBuilder(builders.NewFieldBuilder("clean_hash", "string").
		WithTraits("readable", "writable").
		WithSemanticType("digest"))

	b.AddFieldBuilder(builders.NewFieldBuilder("raw_content", "string").
		WithTraits("readable", "writable").
		WithSemanticType("statement"))

	b.AddFieldBuilder(builders.NewFieldBuilder("clean_content", "string").
		WithTraits("readable", "writable").
		WithSemanticType("statement"))

	b.AddFieldBuilder(builders.NewFieldBuilder("fetched_at", "string").
		WithTraits("readable", "writable").
		WithSemanticType("time"))
}

func (b *IoCacheBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

func (b *IoCacheBuilder) GetVersion() string {
	return "v2_0_0"
}

func (b *IoCacheBuilder) GetOntology() string {
	return "io_cache"
}

func init() {
	builders.RegisterBuilder(NewIoCacheBuilder())
}
