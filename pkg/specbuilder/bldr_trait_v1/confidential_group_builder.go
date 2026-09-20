package bldr_trait_v1

import (
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
)

// ConfidentialGroupBuilder builds the confidential_group trait at version v1_0_0
// File: bldr_trait_v1/confidential_group_builder.go - version is encoded in package/directory name
type ConfidentialGroupBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewConfidentialGroupBuilder creates a new builder for confidential_group trait version v1_0_0
func NewConfidentialGroupBuilder() *ConfidentialGroupBuilder {
	builder := &ConfidentialGroupBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("confidential_group", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Trait group for confidential objects that require access:confidential.\\nExtends base_object_traits with the same trait set but indicates objects\\nwith restricted access requirements. Use for objects containing sensitive data.\\nThis is primarily a semantic marker - it includes all base_object_traits.\\n").
		SetCategory("specialized-group").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("base_object_traits").
		AddIncludes("base_object_traits")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewConfidentialGroupBuilder())
}
