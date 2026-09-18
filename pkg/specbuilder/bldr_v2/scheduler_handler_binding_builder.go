package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// SchedulerHandlerBindingBuilder builds the scheduler_handler_binding spec at version v2_0_0
// File: bldr_v2/scheduler_handler_binding_builder.go - version is encoded in package/directory name
type SchedulerHandlerBindingBuilder struct {
	*builders.BaseSpecBuilder
}

// NewSchedulerHandlerBindingBuilder creates a new builder for scheduler_handler_binding spec version v2_0_0
func NewSchedulerHandlerBindingBuilder() *SchedulerHandlerBindingBuilder {
	builder := &SchedulerHandlerBindingBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("scheduler_handler_binding", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Optional dynamic binding overlay for scheduler job handler dispatch.\\nEach object can remap one scheduler job_type to a handler_key during cache prewarm.\\nStatic code defaults remain the fallback when no valid binding exists.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("base_object_traits")

	// Add fields
	builder.addSchedulerHandlerBindingFields()

	return builder
}

// addSchedulerHandlerBindingFields adds the scheduler_handler_binding fields
func (b *SchedulerHandlerBindingBuilder) addSchedulerHandlerBindingFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("enabled", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("when false, binding is ignored.").
			Cardinality("one").
			Criticality("association").
			Default(true).
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Whether this binding is active.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("boolean.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("state").
		WithProfileCode("SHB-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("handler_key", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("maps to a compiled handler constructor key in scheduler registry.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("scheduler handler registry.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Stable handler key used by prewarm overlay resolution.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"cache_prewarm",
				"cache_invalidation",
				"retention_tolerance",
				"maintenance",
				"scheduler_job_retention",
				"autofix_batch_cleanup",
				"cleanup",
				"lifecycle_check",
				"integrity_check",
				"object_validation",
				"audit_event_aggregation",
				"change_journal_aggregation",
				"aggregation_metrics_cleanup",
				"generic_metrics_cleanup",
				"metrics_collection",
				"scheduler_events_aggregation",
				"cascade_update",
				"operation_execution",
				"run_wrapper",
				"callback_listener",
				"test_io",
				"context_refresh",
				"convergence_session_tick",
				"data_cell_envelope_tick",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SHB-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("job_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("selects which scheduler job_type this binding applies to.").
			Cardinality("one").
			Criticality("composition").
			Default(nil).
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Scheduler job_type to bind (must be a known scheduler job_type value).").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("enum.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"context_refresh",
				"manifest_snapshot",
				"test_runner",
				"aggregation",
				"lifecycle_check",
				"audit_event_aggregation",
				"change_journal_aggregation",
				"integrity_check",
				"cache_prewarm",
				"cache_invalidation",
				"cascade_update",
				"operation_execution",
				"run_wrapper",
				"callback_listener",
				"metrics_collection",
				"retention_tolerance",
				"scheduler_job_retention",
				"autofix_batch_cleanup",
				"aggregation_metrics_cleanup",
				"generic_metrics_cleanup",
				"maintenance",
				"object_validation",
				"scheduler_events_aggregation",
				"cleanup",
				"convergence_session_tick",
				"data_cell_envelope_tick",
			}).
			Required(true).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SHB-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("notes", "text").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional rationale/context for the binding.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("text.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("SHB-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("priority", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("admin.").
			AutomationHooks("deterministic conflict resolution (lower wins).").
			Cardinality("one").
			Criticality("association").
			Default(1000).
			Dependencies("scheduler.").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Lower value is applied first when multiple bindings target one job_type.").
			Security("non-sensitive").
			SystemUsage([]any{
				"scheduler",
			}).
			Validation("integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential", "role:admin").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "modifiable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("quantity").
		WithProfileCode("SHB-004"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *SchedulerHandlerBindingBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *SchedulerHandlerBindingBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *SchedulerHandlerBindingBuilder) GetOntology() string {
	return "scheduler_handler_binding"
}

func init() {
	builders.RegisterBuilder(NewSchedulerHandlerBindingBuilder())
}
