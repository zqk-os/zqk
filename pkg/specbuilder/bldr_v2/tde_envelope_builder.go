package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// TdeEnvelopeBuilder builds the tde_envelope spec at version v2_0_0
// File: bldr_v2/tde_envelope_builder.go - version is encoded in package/directory name
type TdeEnvelopeBuilder struct {
	*builders.BaseSpecBuilder
}

// NewTdeEnvelopeBuilder creates a new builder for tde_envelope spec version v2_0_0
func NewTdeEnvelopeBuilder() *TdeEnvelopeBuilder {
	builder := &TdeEnvelopeBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("tde_envelope", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("null").
		SetDescription("Envelope for time-delayed execution of agent intents.").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	return builder
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *TdeEnvelopeBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *TdeEnvelopeBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *TdeEnvelopeBuilder) GetOntology() string {
	return "tde_envelope"
}

func init() {
	builders.RegisterBuilder(NewTdeEnvelopeBuilder())
}
