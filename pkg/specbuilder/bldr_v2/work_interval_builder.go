package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// WorkIntervalBuilder builds the work_interval mixin spec at version v2_0_0.
type WorkIntervalBuilder struct {
	*builders.BaseSpecBuilder
}

// NewWorkIntervalBuilder creates a builder for the work_interval mixin.
func NewWorkIntervalBuilder() *WorkIntervalBuilder {
	builder := &WorkIntervalBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("work_interval", "v2_0_0"),
	}

	builder.
		SetExtends("base_object").
		SetDescription("Mixin for kinds that participate in a work clock (started_at, completed_at). completed_at is done-of-work, not archive-of-record. Not instantiable (kind_mappings skip_specs). TRACK: BLI-KERNEL-WORK-ENVELOPE-001 / WORK_ENVELOPE_AND_EFFORT_FACETS.md / CRIT-KERNEL-WORK-ENVELOPE-BASE-LIFT-001. ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("completable")

	builder.addWorkIntervalFields()
	return builder
}

func (b *WorkIntervalBuilder) addWorkIntervalFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("completed_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner").
			AutomationHooks("stamped by the transition membrane on work_done.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("lifecycle transitions.").
			Lifecycle("mutable (set once on work-done; not overwritten on later field edits)").
			Observability("yes.").
			Purpose("ISO-8601 datetime when work-done was reached.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"metrics",
				"wall-clock actual_effort bound",
			}).
			Validation("ISO-8601 datetime format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WIN-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("started_at", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation/owner").
			AutomationHooks("stamped by the transition membrane on execution-locked hops.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("lifecycle transitions.").
			Lifecycle("mutable (set once on first execution-locked hop)").
			Observability("yes.").
			Purpose("ISO-8601 datetime when execution of this work interval began.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"metrics",
				"wall-clock actual_effort bound",
			}).
			Validation("ISO-8601 datetime format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("WIN-001"))
}

func (b *WorkIntervalBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

func (b *WorkIntervalBuilder) GetVersion() string {
	return "v2_0_0"
}

func (b *WorkIntervalBuilder) GetOntology() string {
	return "work_interval"
}

func init() {
	builders.RegisterBuilder(NewWorkIntervalBuilder())
}
