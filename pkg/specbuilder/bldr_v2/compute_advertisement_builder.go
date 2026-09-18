package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// ComputeAdvertisementBuilder builds the compute_advertisement spec at version v2_0_0
// File: bldr_v2/compute_advertisement_builder.go - version is encoded in package/directory name
type ComputeAdvertisementBuilder struct {
	*builders.BaseSpecBuilder
}

// NewComputeAdvertisementBuilder creates a new builder for compute_advertisement spec version v2_0_0
func NewComputeAdvertisementBuilder() *ComputeAdvertisementBuilder {
	builder := &ComputeAdvertisementBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("compute_advertisement", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Used by Tool Pods to broadcast their specific heavy compute capabilities (e.g., ffmpeg-stitcher, ml-inference) to the mesh marketplace.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addComputeAdvertisementFields()

	return builder
}

// addComputeAdvertisementFields adds the compute_advertisement fields
func (b *ComputeAdvertisementBuilder) addComputeAdvertisementFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("capability_type", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("endpoint", "string"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metadata", "map"))
	b.AddFieldBuilder(builders.NewFieldBuilder("provider_kernel_ref", "string"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ComputeAdvertisementBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ComputeAdvertisementBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ComputeAdvertisementBuilder) GetOntology() string {
	return "compute_advertisement"
}

func init() {
	builders.RegisterBuilder(NewComputeAdvertisementBuilder())
}
