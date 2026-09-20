package bldr_trait_v1

import (
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
)

// FieldReferenceGroupBuilder builds the field_reference_group trait at version v1_0_0
// File: bldr_trait_v1/field_reference_group_builder.go - version is encoded in package/directory name
type FieldReferenceGroupBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewFieldReferenceGroupBuilder creates a new builder for field_reference_group trait version v1_0_0
func NewFieldReferenceGroupBuilder() *FieldReferenceGroupBuilder {
	builder := &FieldReferenceGroupBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("field_reference_group", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Trait group for reference fields (foreign keys, object references).\\nThese fields reference other objects and are commonly filtered but not typically\\nuser-editable (references are usually set by system or through relationships).\\n").
		SetCategory("specialized-group").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("readable").
		AddIncludes("readable").
		AddIncludes("filterable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewFieldReferenceGroupBuilder())
}
