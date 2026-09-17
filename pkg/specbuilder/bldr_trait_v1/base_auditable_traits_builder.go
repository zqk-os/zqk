package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// BaseAuditableTraitsBuilder builds the base_auditable_traits trait at version v1_0_0
// File: bldr_trait_v1/base_auditable_traits_builder.go - version is encoded in package/directory name
type BaseAuditableTraitsBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewBaseAuditableTraitsBuilder creates a new builder for base_auditable_traits trait version v1_0_0
func NewBaseAuditableTraitsBuilder() *BaseAuditableTraitsBuilder {
	builder := &BaseAuditableTraitsBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("base_auditable_traits", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Base trait group for all objects extending auditable.\\nThis trait group includes the standard traits that all auditable objects inherit.\\nObjects extending auditable automatically get these traits.\\nWhen this trait group is referenced, it expands to include all listed traits.\\n").
		SetCategory("base-group").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddIncludes("listable").
		AddIncludes("readable").
		AddIncludes("writable").
		AddIncludes("modifiable").
		AddIncludes("removable").
		AddIncludes("formatable").
		AddIncludes("filterable").
		AddIncludes("sortable").
		AddIncludes("searchable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewBaseAuditableTraitsBuilder())
}
