package bldr_trait_v1

import (
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
)

// GroupableBuilder builds the groupable trait at version v1_0_0
// File: bldr_trait_v1/groupable_builder.go - version is encoded in package/directory name
type GroupableBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewGroupableBuilder creates a new builder for groupable trait version v1_0_0
func NewGroupableBuilder() *GroupableBuilder {
	builder := &GroupableBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("groupable", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Object/field can be grouped in collections").
		SetCategory("standard").
		SetObjectLevel(false).
		SetFieldLevel(false)

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewGroupableBuilder())
}
