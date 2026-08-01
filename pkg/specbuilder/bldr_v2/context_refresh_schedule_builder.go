package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// ContextRefreshScheduleBuilder builds the context_refresh_schedule spec at version v2_0_0
// File: bldr_v2/context_refresh_schedule_builder.go - version is encoded in package/directory name
type ContextRefreshScheduleBuilder struct {
	*builders.BaseSpecBuilder
}

// NewContextRefreshScheduleBuilder creates a new builder for context_refresh_schedule spec version v2_0_0
func NewContextRefreshScheduleBuilder() *ContextRefreshScheduleBuilder {
	builder := &ContextRefreshScheduleBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("context_refresh_schedule", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Defines how often \\\\\".cursor\\\\\" or other context artifacts are refreshed, per profile/project (scheduler schedule, not a policy). ").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addContextRefreshScheduleFields()

	return builder
}

// addContextRefreshScheduleFields adds the context_refresh_schedule fields
func (b *ContextRefreshScheduleBuilder) addContextRefreshScheduleFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("cadence", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("owner.").
			AutomationHooks("used to configure scheduler jobs.").
			Cardinality("one").
			Criticality("composition").
			Default("P1D").
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Desired refresh interval (cron or ISO duration).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("cron/ISO format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("field_mutable_group").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("CRP-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("last_refresh", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used to determine if refresh is needed.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduler.").
			Lifecycle("mutable (set by scheduler).").
			Observability("yes").
			Purpose("Last time context was refreshed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"compliance",
			}).
			Validation("ISO-8601 datetime.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("readable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CRP-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("next_refresh", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation.").
			AutomationHooks("used for scheduling.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("scheduler.").
			Lifecycle("mutable (recalculated by scheduler).").
			Observability("yes").
			Purpose("Next scheduled refresh time.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduling",
			}).
			Validation("ISO-8601 datetime.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).
			Required(false).
			Build()).
		WithTraits("field_read_only_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CRP-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("target", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin/owner.").
			AutomationHooks("ensures correct subject.").
			Cardinality("one").
			Criticality("composition").
			Default("required").
			Dependencies("scheduler job creation.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("What the schedule applies to (\\\\\\\"profile:{id}\\\\\\\", \\\\\\\"project:{id}\\\\\\\", etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("reference pattern.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("CRP-001"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *ContextRefreshScheduleBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *ContextRefreshScheduleBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *ContextRefreshScheduleBuilder) GetOntology() string {
	return "context_refresh_schedule"
}

func init() {
	builders.RegisterBuilder(NewContextRefreshScheduleBuilder())
}
