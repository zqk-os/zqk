package bldr_trait_v1

import (
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
)

// FilterableBuilder builds the filterable trait at version v1_0_0
// File: bldr_trait_v1/filterable_builder.go - version is encoded in package/directory name
type FilterableBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewFilterableBuilder creates a new builder for filterable trait version v1_0_0
func NewFilterableBuilder() *FilterableBuilder {
	builder := &FilterableBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("filterable", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Object/field can be filtered in queries").
		SetCategory("standard").
		SetObjectLevel(false).
		SetFieldLevel(false)

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewFilterableBuilder())
}
