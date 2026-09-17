package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// SnapableBuilder builds the snapable trait at version v1_0_0
// File: bldr_trait_v1/snapable_builder.go - version is encoded in package/directory name
type SnapableBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewSnapableBuilder creates a new builder for snapable trait version v1_0_0
func NewSnapableBuilder() *SnapableBuilder {
	builder := &SnapableBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("snapable", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Trait indicating that an object or field can participate in snapshot operations.\\nObjects with this trait can report their state at a specific timestamp.\\nFields with this trait can be included or excluded from snapshots.\\n\\nAt the object level, defines query specifications for discovering objects to snapshot.\\nAt the field level, defines data handlers for processing field data during snapshot.\\n").
		SetCategory("system").
		SetObjectLevel(false).
		SetFieldLevel(false).
		AddRequires("readable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewSnapableBuilder())
}
