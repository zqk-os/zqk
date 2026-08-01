package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// InfrastructureAdapterBuilder builds the infrastructure_adapter spec at version v2_0_0
// File: bldr_v2/infrastructure_adapter_builder.go - version is encoded in package/directory name
type InfrastructureAdapterBuilder struct {
	*builders.BaseSpecBuilder
}

// NewInfrastructureAdapterBuilder creates a new builder for infrastructure_adapter spec version v2_0_0
func NewInfrastructureAdapterBuilder() *InfrastructureAdapterBuilder {
	builder := &InfrastructureAdapterBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("infrastructure_adapter", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents an external specialized utility (Kafka, Flink, Kinesis) that the kernel \\nproxies high-volume workloads to. Defines connection metadata and capability limits.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addInfrastructureAdapterFields()

	return builder
}

// addInfrastructureAdapterFields adds the infrastructure_adapter fields
func (b *InfrastructureAdapterBuilder) addInfrastructureAdapterFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("assigned_specialization", "string").
		WithTraits("readable", "writable", "groupable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("credentials_ref", "string").
		WithTraits("readable", "writable").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("endpoint", "string").
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("proxy_mode", "string").
		WithDefault("buffered_bridge").
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("throughput_capacity", "float").
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("units", "string").
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("utility_type", "string").
		WithTraits("readable", "writable", "groupable"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *InfrastructureAdapterBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *InfrastructureAdapterBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *InfrastructureAdapterBuilder) GetOntology() string {
	return "infrastructure_adapter"
}

func init() {
	builders.RegisterBuilder(NewInfrastructureAdapterBuilder())
}
