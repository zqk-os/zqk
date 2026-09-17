package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// OccupancyBuilder builds the occupancy mixin spec at version v2_0_0.
type OccupancyBuilder struct {
	*builders.BaseSpecBuilder
}

// NewOccupancyBuilder creates a builder for the occupancy mixin.
func NewOccupancyBuilder() *OccupancyBuilder {
	builder := &OccupancyBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("occupancy", "v2_0_0"),
	}

	builder.
		SetExtends("work_interval").
		SetDescription("Mixin for an occupancy slot (claimed_by, claimed_at). Occupiable means the slot exists; empty claimed_by means unoccupied — including in_progress, which is parked. Filling the slot is exclusive claim. Assignment (persona_refs) is routing and does not occupy. Occupancy is not a timesheet: it extends work_interval (Gantt/clock plane), not work_unit (effort). Not instantiable (kind_mappings skip_specs). Inherit: Gantt bodies that can be occupied (priority_plan) switch extends to occupancy. Compose: timesheet kinds that also need occupancy (agent_task) keep extends: work_unit and list occupancy under composes. TRACK: POL-KERNEL-GANTT-OCCUPANCY-001 / WFL-MULTI-AGENT-WORK-CLAIM. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("occupiable")

	builder.addOccupancyFields()
	return builder
}

func (b *OccupancyBuilder) addOccupancyFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder(FieldClaimedAt, "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("system.").
			AutomationHooks("stamped with claimed_by on successful claim.").
			Cardinality("one").
			Criticality("metadata").
			Default(nil).
			Dependencies("claimed_by.").
			Lifecycle("mutable.").
			Observability("yes.").
			Purpose("RFC3339 timestamp when the current claim was taken.").
			Security("non-sensitive").
			SystemUsage([]any{
				"observability",
				"stale-claim detection",
			}).
			Validation("RFC3339 datetime string.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("datetime").
		WithProfileCode("OCC-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder(FieldClaimedBy, "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("swarm worker / orchestrator.").
			AutomationHooks("WFL-MULTI-AGENT-WORK-CLAIM atomic claim; ATK fail-closed via pkg/agentclaim.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("account or agent seat id.").
			Lifecycle("mutable.").
			Observability("yes.").
			Purpose("Agent or account id holding the exclusive execution claim on this object.").
			Security("non-sensitive").
			SystemUsage([]any{
				"routing",
				"deduplication",
			}).
			Validation("non-empty string when claimed.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "modifiable", "readable", "searchable", "writable").
		WithPermissions("rwx").
		WithSemanticType("reference").
		WithProfileCode("OCC-001"))
}

func (b *OccupancyBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

func (b *OccupancyBuilder) GetVersion() string {
	return "v2_0_0"
}

func (b *OccupancyBuilder) GetOntology() string {
	return "occupancy"
}

func init() {
	builders.RegisterBuilder(NewOccupancyBuilder())
}
