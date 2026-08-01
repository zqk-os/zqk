package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// BaseObjectTraitsBuilder builds the base_object_traits trait at version v1_0_0
// File: bldr_trait_v1/base_object_traits_builder.go - version is encoded in package/directory name
type BaseObjectTraitsBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewBaseObjectTraitsBuilder creates a new builder for base_object_traits trait version v1_0_0
func NewBaseObjectTraitsBuilder() *BaseObjectTraitsBuilder {
	builder := &BaseObjectTraitsBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("base_object_traits", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Base trait group for all objects extending base_object.\\nThis trait group includes base_auditable_traits, groupable, and auto_status_transitionable.\\nObjects extending base_object automatically get these traits.\\nWhen this trait group is referenced, it expands to include all auditable traits plus shared object behavior traits.\\n").
		SetCategory("base-group").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("base_auditable_traits").
		AddIncludes("base_auditable_traits").
		AddIncludes("groupable").
		AddIncludes("auto_status_transitionable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewBaseObjectTraitsBuilder())
}
