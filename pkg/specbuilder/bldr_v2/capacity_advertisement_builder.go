package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// CapacityAdvertisementBuilder builds the capacity_advertisement spec at version v2_0_0
// File: bldr_v2/capacity_advertisement_builder.go - version is encoded in package/directory name
type CapacityAdvertisementBuilder struct {
	*builders.BaseSpecBuilder
}

// NewCapacityAdvertisementBuilder creates a new builder for capacity_advertisement spec version v2_0_0
func NewCapacityAdvertisementBuilder() *CapacityAdvertisementBuilder {
	builder := &CapacityAdvertisementBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("capacity_advertisement", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Represents a broadcast advertisement of available resources (compute, skill, storage)\\nthat a kernel is willing to lease to peers in the Sovereign Mesh.\\n").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addCapacityAdvertisementFields()

	return builder
}

// addCapacityAdvertisementFields adds the capacity_advertisement fields
func (b *CapacityAdvertisementBuilder) addCapacityAdvertisementFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("availability_window", "string").
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("provider_kernel_ref", "string").
		WithTraits("field_reference_group", "writable").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("quantity", "float").
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("resource_id", "string").
		WithTraits("readable", "writable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("resource_type", "string").
		WithTraits("readable", "writable", "groupable"))
	b.AddFieldBuilder(builders.NewFieldBuilder("terms_ref", "string").
		WithTraits("field_reference_group", "writable").
		WithSemanticType("reference"))
	b.AddFieldBuilder(builders.NewFieldBuilder("units", "string").
		WithTraits("readable", "writable"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *CapacityAdvertisementBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *CapacityAdvertisementBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *CapacityAdvertisementBuilder) GetOntology() string {
	return "capacity_advertisement"
}

func init() {
	builders.RegisterBuilder(NewCapacityAdvertisementBuilder())
}
