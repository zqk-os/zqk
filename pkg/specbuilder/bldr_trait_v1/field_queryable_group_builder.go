package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// FieldQueryableGroupBuilder builds the field_queryable_group trait at version v1_0_0
// File: bldr_trait_v1/field_queryable_group_builder.go - version is encoded in package/directory name
type FieldQueryableGroupBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewFieldQueryableGroupBuilder creates a new builder for field_queryable_group trait version v1_0_0
func NewFieldQueryableGroupBuilder() *FieldQueryableGroupBuilder {
	builder := &FieldQueryableGroupBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("field_queryable_group", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Trait group for fields that are commonly used in queries, filters, and sorting.\\nIncludes all query-related traits for fields that are frequently searched or filtered.\\nUse for identifier fields, timestamps, status fields, and other commonly queried fields.\\n").
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
	trait_builders.RegisterBuilder(NewFieldQueryableGroupBuilder())
}
