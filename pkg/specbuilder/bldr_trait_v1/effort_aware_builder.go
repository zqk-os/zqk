package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// EffortAwareBuilder builds the effort_aware trait at version v1_0_0
// File: bldr_trait_v1/effort_aware_builder.go - version is encoded in package/directory name
type EffortAwareBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewEffortAwareBuilder creates a new builder for effort_aware trait version v1_0_0
func NewEffortAwareBuilder() *EffortAwareBuilder {
	builder := &EffortAwareBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("effort_aware", "v1_0_0"),
	}

	// Configure the trait
	builder.
		SetDescription("Object-level planned-vs-realized cost trait (estimated_effort, actual_effort).\\nComposes completable via includes: listing effort_aware is sufficient; do not\\nalso list completable on the object spec. Actual effort is only lawful against\\nthe work clock.\\nTRACK: BLI-KERNEL-WORK-ENVELOPE-001 / WORK_ENVELOPE_AND_EFFORT_FACETS.md.\\n").
		SetCategory("behavior").
		SetObjectLevel(true).
		SetFieldLevel(false).
		AddRequires("completable").
		AddIncludes("completable")

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewEffortAwareBuilder())
}
