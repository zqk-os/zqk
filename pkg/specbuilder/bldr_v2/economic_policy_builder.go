package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// EconomicPolicyBuilder builds the economic_policy spec at version v2_0_0
// File: bldr_v2/economic_policy_builder.go - version is encoded in package/directory name
type EconomicPolicyBuilder struct {
	*builders.BaseSpecBuilder
}

// NewEconomicPolicyBuilder creates a new builder for economic_policy spec version v2_0_0
func NewEconomicPolicyBuilder() *EconomicPolicyBuilder {
	builder := &EconomicPolicyBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("economic_policy", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines the economic heuristics for kernel-to-kernel interaction.\\nControls resource pricing, credit limits, and settlement terms.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addEconomicPolicyFields()

	return builder
}

// addEconomicPolicyFields adds the economic_policy fields
func (b *EconomicPolicyBuilder) addEconomicPolicyFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("cost_per_query", "integer").
		WithDefault(1).
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("max_credit_limit", "integer").
		WithDefault(1000).
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("settlement_currency", "string").
		WithDefault("ZQK_CREDIT").
		WithTraits("readable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("storage_rent_per_mb", "integer").
		WithDefault(10).
		WithTraits("readable", "writable"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *EconomicPolicyBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *EconomicPolicyBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *EconomicPolicyBuilder) GetOntology() string {
	return "economic_policy"
}

func init() {
	builders.RegisterBuilder(NewEconomicPolicyBuilder())
}
