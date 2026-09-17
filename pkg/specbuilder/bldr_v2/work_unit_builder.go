package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// WorkUnitBuilder builds the work_unit mixin spec at version v2_0_0.
type WorkUnitBuilder struct {
	*builders.BaseSpecBuilder
}

// NewWorkUnitBuilder creates a builder for the work_unit mixin.
func NewWorkUnitBuilder() *WorkUnitBuilder {
	builder := &WorkUnitBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("work_unit", "v2_0_0"),
	}

	builder.
		SetExtends("work_interval").
		SetDescription("Mixin for timesheet kinds: estimated_effort (planner intent) plus actual_effort (membrane-autofilled against the work clock). Not instantiable (kind_mappings skip_specs). TRACK: BLI-KERNEL-WORK-ENVELOPE-001 / WORK_ENVELOPE_AND_EFFORT_FACETS.md / CRIT-KERNEL-WORK-ENVELOPE-BASE-LIFT-001. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("effort_aware")

	builder.addWorkUnitFields()
	return builder
}

func (b *WorkUnitBuilder) addWorkUnitFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("actual_effort", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/automation").
			AutomationHooks("used for velocity calculations and estimation accuracy analysis.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("time tracking, metrics collection.").
			Lifecycle("mutable (updated as work progresses)").
			Observability("yes.").
			Purpose("Actual effort expended (e.g., \\\\\\\"5.2 days\\\\\\\", \\\\\\\"32 hours\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"metrics",
				"retrospectives",
			}).
			Validation("Free-form string describing actual effort.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("estimated_effort", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner/planner").
			AutomationHooks("used for timeline projections and capacity planning.").
			Cardinality("one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduling, timeline generation.").
			Lifecycle("mutable").
			Observability("yes.").
			Purpose("Estimated effort required (e.g., \\\\\\\"4-6 weeks\\\\\\\", \\\\\\\"2 days\\\\\\\", \\\\\\\"8 hours\\\\\\\", \\\\\\\"5 story points\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"planning",
				"scheduling",
				"resource allocation",
			}).
			Validation("Free-form string describing effort estimate.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BSE-012"))
}

func (b *WorkUnitBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

func (b *WorkUnitBuilder) GetVersion() string {
	return "v2_0_0"
}

func (b *WorkUnitBuilder) GetOntology() string {
	return "work_unit"
}

func init() {
	builders.RegisterBuilder(NewWorkUnitBuilder())
}
