package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// ConstrainableBuilder builds the constrainable trait at version v1_0_0
// File: bldr_trait_v1/constrainable_builder.go - version is encoded in package/directory name
type ConstrainableBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewConstrainableBuilder creates a new builder for constrainable trait version v1_0_0
func NewConstrainableBuilder() *ConstrainableBuilder {
	builder := &ConstrainableBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("constrainable", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Object can have layout/constraint rules applied (domain-specific)").
		SetCategory("domain-specific").
		SetObjectLevel(false).
		SetFieldLevel(false)

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewConstrainableBuilder())
}
