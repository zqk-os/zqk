package bldr_trait_v1

import (
	"github.com/lanceman/zqk/pkg/specbuilder/trait_builders"
)

// OccupiableBuilder builds the occupiable trait at version v1_0_0
type OccupiableBuilder struct {
	*trait_builders.BaseTraitBuilder
}

// NewOccupiableBuilder creates a new builder for occupiable trait version v1_0_0
func NewOccupiableBuilder() *OccupiableBuilder {
	builder := &OccupiableBuilder{
		BaseTraitBuilder: trait_builders.NewBaseTraitBuilder("occupiable", "v1_0_0"),
	}

	builder.
		SetDescription("Object-level occupancy-slot trait (claimed_by, claimed_at).\\nThe slot exists; it is either open or held. Empty claimed_by means unoccupied — including in_progress, which is parked, not\\n\"someone is working.\" Filling the slot is exclusive claim. Assignment (persona_refs) is routing and does not occupy.\\nOccupancy is not a lifecycle status, not Gantt rank\\n(priority_plan.active_order), and not a timesheet (work_unit / effort_aware).\\nFields live on the occupancy mixin. TRACK: POL-KERNEL-GANTT-OCCUPANCY-001.\\n").
		SetCategory("behavior").
		SetObjectLevel(true).
		SetFieldLevel(false)

	return builder
}

func init() {
	trait_builders.RegisterBuilder(NewOccupiableBuilder())
}
