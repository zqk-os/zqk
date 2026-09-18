package bldr_trait_v1

import (
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
)

// FieldDisplayGroupBuilder builds the field_display_group trait at version v1_0_0
// File: bldr_trait_v1/field_display_group_builder.go - version is encoded in package/directory name
type FieldDisplayGroupBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewFieldDisplayGroupBuilder creates a new builder for field_display_group trait version v1_0_0
func NewFieldDisplayGroupBuilder() *FieldDisplayGroupBuilder {
	builder := &FieldDisplayGroupBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("field_display_group", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Trait group for fields that are commonly displayed in lists and tables.\\nIncludes listable and formatable for fields that appear in UI displays.\\nUse for title, description, status, and other display-oriented fields.\\n").
		SetCategory("specialized-group").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("readable").
		AddIncludes("readable").
		AddIncludes("listable").
		AddIncludes("formatable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewFieldDisplayGroupBuilder())
}
