package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// CompressionPolicyBuilder builds the compression_policy spec at version v2_0_0
// File: bldr_v2/compression_policy_builder.go - version is encoded in package/directory name
type CompressionPolicyBuilder struct {
	*builders.BaseSpecBuilder
}

// NewCompressionPolicyBuilder creates a new builder for compression_policy spec version v2_0_0
func NewCompressionPolicyBuilder() *CompressionPolicyBuilder {
	builder := &CompressionPolicyBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("compression_policy", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines how the High-Throughput Semantic (HTS) codec should compress objects\\nfor mesh-wide transfer. Leverages shared ontologies to omit redundant fields.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addCompressionPolicyFields()

	return builder
}

// addCompressionPolicyFields adds the compression_policy fields
func (b *CompressionPolicyBuilder) addCompressionPolicyFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("algorithm", "string").
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("omit_schema_defaults", "bool").
		WithDefault(true).
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("shared_ontology_refs", "list").
		WithTraits("readable", "writable").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target_kind", "string").
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("threshold_bytes", "integer").
		WithDefault(1024).
		WithTraits("readable", "writable"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *CompressionPolicyBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *CompressionPolicyBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *CompressionPolicyBuilder) GetOntology() string {
	return "compression_policy"
}

func init() {
	builders.RegisterBuilder(NewCompressionPolicyBuilder())
}
