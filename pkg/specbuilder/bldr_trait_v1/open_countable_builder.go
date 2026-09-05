package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// OpenCountableBuilder builds the open_countable trait at version v1_0_0.
type OpenCountableBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewOpenCountableBuilder creates a builder for open_countable trait version v1_0_0.
func NewOpenCountableBuilder() *OpenCountableBuilder {
	builder := &OpenCountableBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("open_countable", "v1_0_0"),
	}

	builder.
		SetDescription("Object-level remaining-open capability. The container's behavior changes when remaining_open_count hits zero. Fields live on the remaining_open mixin. Parallel: occupiable vs occupancy. TRACK: POL-ARCH-20260901 / BLI-CEF-CONTAINER-REMAINING-OPEN-001.\\n").
		SetCategory("behavior").
		SetObjectLevel(true).
		SetFieldLevel(false).
		SetConfig(map[string]any{"count_field": "remaining_open_count"})

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewOpenCountableBuilder())
}
