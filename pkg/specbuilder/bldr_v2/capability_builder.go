package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// CapabilityBuilder builds the capability spec at version v2_0_0
// File: bldr_v2/capability_builder.go - version is encoded in package/directory name
type CapabilityBuilder struct {
	*builders.BaseSpecBuilder
}

// NewCapabilityBuilder creates a new builder for capability spec version v2_0_0
func NewCapabilityBuilder() *CapabilityBuilder {
	builder := &CapabilityBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("capability", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines a synthesized capability.").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addCapabilityFields()

	return builder
}

// addCapabilityFields adds the capability fields
func (b *CapabilityBuilder) addCapabilityFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("signature", "string").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *CapabilityBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *CapabilityBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *CapabilityBuilder) GetOntology() string {
	return "capability"
}

func init() {
	builders.RegisterBuilder(NewCapabilityBuilder())
}
