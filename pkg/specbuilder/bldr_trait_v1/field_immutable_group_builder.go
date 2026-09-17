package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// FieldImmutableGroupBuilder builds the field_immutable_group trait at version v1_0_0
// File: bldr_trait_v1/field_immutable_group_builder.go - version is encoded in package/directory name
type FieldImmutableGroupBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewFieldImmutableGroupBuilder creates a new builder for field_immutable_group trait version v1_0_0
func NewFieldImmutableGroupBuilder() *FieldImmutableGroupBuilder {
	builder := &FieldImmutableGroupBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("field_immutable_group", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Trait group for immutable fields that are set once and never change.\\nTypically used for system-generated identifiers, timestamps, and other fields\\nwith lifecycle: immutable. These fields are readable and queryable but not modifiable.\\n").
		SetCategory("specialized-group").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("readable").
		AddIncludes("readable").
		AddIncludes("listable").
		AddIncludes("filterable").
		AddIncludes("sortable").
		AddIncludes("searchable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewFieldImmutableGroupBuilder())
}
