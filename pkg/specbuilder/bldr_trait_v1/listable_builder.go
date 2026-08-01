package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// ListableBuilder builds the listable trait at version v1_0_0
// File: bldr_trait_v1/listable_builder.go - version is encoded in package/directory name
type ListableBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewListableBuilder creates a new builder for listable trait version v1_0_0
func NewListableBuilder() *ListableBuilder {
	builder := &ListableBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("listable", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Object can be listed/queried in collections").
		SetCategory("standard").
		SetObjectLevel(false).
		SetFieldLevel(false)

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewListableBuilder())
}
