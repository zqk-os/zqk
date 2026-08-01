package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// FieldMutableGroupBuilder builds the field_mutable_group trait at version v1_0_0
// File: bldr_trait_v1/field_mutable_group_builder.go - version is encoded in package/directory name
type FieldMutableGroupBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewFieldMutableGroupBuilder creates a new builder for field_mutable_group trait version v1_0_0
func NewFieldMutableGroupBuilder() *FieldMutableGroupBuilder {
	builder := &FieldMutableGroupBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("field_mutable_group", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Trait group for standard mutable fields that can be read, written, and modified.\\nUse for user-editable fields like descriptions, titles, and other content fields.\\n").
		SetCategory("specialized-group").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("readable").
		AddIncludes("readable").
		AddIncludes("writable").
		AddIncludes("modifiable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewFieldMutableGroupBuilder())
}
