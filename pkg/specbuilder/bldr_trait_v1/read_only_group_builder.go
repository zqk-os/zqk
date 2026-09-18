package bldr_trait_v1

import (
	"github.com/zqk-os/zqk/pkg/specbuilder/trait_builders"
)

// ReadOnlyGroupBuilder builds the read_only_group trait at version v1_0_0
// File: bldr_trait_v1/read_only_group_builder.go - version is encoded in package/directory name
type ReadOnlyGroupBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewReadOnlyGroupBuilder creates a new builder for read_only_group trait version v1_0_0
func NewReadOnlyGroupBuilder() *ReadOnlyGroupBuilder {
	builder := &ReadOnlyGroupBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("read_only_group", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Trait group for read-only objects that cannot be modified or deleted.\\nExtends base_object_traits but excludes writable, modifiable, and removable.\\nUse for objects that are system-generated or immutable (e.g., audit events, metrics).\\n").
		SetCategory("specialized-group").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("base_object_traits").
		AddIncludes("listable").
		AddIncludes("readable").
		AddIncludes("formatable").
		AddIncludes("groupable").
		AddIncludes("filterable").
		AddIncludes("sortable").
		AddIncludes("searchable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewReadOnlyGroupBuilder())
}
