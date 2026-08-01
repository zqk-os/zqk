package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// ModifiableBuilder builds the modifiable trait at version v1_0_0
// File: bldr_trait_v1/modifiable_builder.go - version is encoded in package/directory name
type ModifiableBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewModifiableBuilder creates a new builder for modifiable trait version v1_0_0
func NewModifiableBuilder() *ModifiableBuilder {
	builder := &ModifiableBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("modifiable", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Object/field can be modified/updated").
		SetCategory("standard").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("readable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewModifiableBuilder())
}
