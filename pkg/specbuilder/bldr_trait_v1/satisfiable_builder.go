package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// SatisfiableBuilder builds the satisfiable trait at version v1_0_0
// File: bldr_trait_v1/satisfiable_builder.go - version is encoded in package/directory name
type SatisfiableBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewSatisfiableBuilder creates a new builder for satisfiable trait version v1_0_0
func NewSatisfiableBuilder() *SatisfiableBuilder {
	builder := &SatisfiableBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("satisfiable", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Object-level predicate trait. Kinds with this trait have a done-state of\\n\"the proposition holds\" (evidence, not duration). Does not compose completable:\\ndo not stamp started_at / completed_at on the satisfaction hop.\\nCategory (acceptance, test, compliance, …) is a field, not a trait.\\nTRACK: BLI-KERNEL-WORK-ENVELOPE-001 / WORK_ENVELOPE_AND_EFFORT_FACETS.md.\\n").
		SetCategory("behavior").
		SetObjectLevel(true).
		SetFieldLevel(false)

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewSatisfiableBuilder())
}
