package bldr_trait_v1

import (
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
)

// RemovableBuilder builds the removable trait at version v1_0_0
// File: bldr_trait_v1/removable_builder.go - version is encoded in package/directory name
type RemovableBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewRemovableBuilder creates a new builder for removable trait version v1_0_0
func NewRemovableBuilder() *RemovableBuilder {
	builder := &RemovableBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("removable", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Object/field can be removed/deleted").
		SetCategory("standard").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("readable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewRemovableBuilder())
}
