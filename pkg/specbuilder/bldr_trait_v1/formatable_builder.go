package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// FormatableBuilder builds the formatable trait at version v1_0_0
// File: bldr_trait_v1/formatable_builder.go - version is encoded in package/directory name
type FormatableBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewFormatableBuilder creates a new builder for formatable trait version v1_0_0
func NewFormatableBuilder() *FormatableBuilder {
	builder := &FormatableBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("formatable", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Object/field can be formatted for display").
		SetCategory("standard").
		SetObjectLevel(false).
		SetFieldLevel(false)

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewFormatableBuilder())
}
