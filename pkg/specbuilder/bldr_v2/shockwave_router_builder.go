package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ShockwaveRouterBuilder builds the shockwave_router spec at version v2_0_0
// File: bldr_v2/shockwave_router_builder.go - version is encoded in package/directory name
type ShockwaveRouterBuilder struct {
	*builders.BaseSpecBuilder
}

// NewShockwaveRouterBuilder creates a new builder for shockwave_router spec version v2_0_0
func NewShockwaveRouterBuilder() *ShockwaveRouterBuilder {
	builder := &ShockwaveRouterBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("shockwave_router", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Micro-API Gateway router for semantic payloads and events.").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addShockwaveRouterFields()

	return builder
}

// addShockwaveRouterFields adds the shockwave_router fields
func (b *ShockwaveRouterBuilder) addShockwaveRouterFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("propagation_rules", "object").
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("reduce_strategy", "object").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
	b.AddFieldBuilder(builders.NewFieldBuilder("trigger_condition", "object").
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ShockwaveRouterBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ShockwaveRouterBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ShockwaveRouterBuilder) GetOntology() string {
	return "shockwave_router"
}

func init() {
	builders.RegisterBuilder(NewShockwaveRouterBuilder())
}
