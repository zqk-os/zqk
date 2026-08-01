package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// SchedulerHealthMetricBuilder builds the scheduler_health_metric spec at version v2_0_0
// File: bldr_v2/scheduler_health_metric_builder.go - version is encoded in package/directory name
type SchedulerHealthMetricBuilder struct {
	*builders.BaseSpecBuilder
}

// NewSchedulerHealthMetricBuilder creates a new builder for scheduler_health_metric spec version v2_0_0
func NewSchedulerHealthMetricBuilder() *SchedulerHealthMetricBuilder {
	builder := &SchedulerHealthMetricBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("scheduler_health_metric", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_metric").
		SetDescription("Scheduler health and recovery metrics. Tracks health check runs, missed job triggers, automatic recoveries, and cron scheduler restarts for monitoring scheduler daemon reliability and self-healing capabilities.\\nLifecycle: scheduler_health_metric_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion).
		AddTrait("listable").
		AddTrait("readable").
		AddTrait("writable").
		AddTrait("modifiable").
		AddTrait("formatable").
		AddTrait("groupable").
		AddTrait("filterable").
		AddTrait("sortable").
		AddTrait("searchable")

	// Add fields
	builder.addSchedulerHealthMetricFields()

	return builder
}

// addSchedulerHealthMetricFields adds the scheduler_health_metric fields
func (b *SchedulerHealthMetricBuilder) addSchedulerHealthMetricFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("cron_restarts", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (scheduler health monitor)").
			AutomationHooks("used for cron stability analysis.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("scheduler health monitoring system.").
			Lifecycle("mutable (incremented when cron scheduler is restarted)").
			Observability("yes").
			Purpose("Total number of times the cron scheduler was automatically restarted.").
			Security("non-sensitive").
			SystemUsage([]any{
				"stability analysis",
				"scheduler reliability reporting",
				"failure detection",
			}).
			Validation("Non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("SHM-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("goroutine_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (scheduler health monitor)").
			AutomationHooks("used for thread explosion diagnosis and trend analysis.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("Go runtime (runtime.NumGoroutine).").
			Lifecycle("mutable (sampled each health check)").
			Observability("yes").
			Purpose("Number of goroutines in the scheduler process at sample time (helps diagnose thread explosion).").
			Security("non-sensitive").
			SystemUsage([]any{
				"thread explosion diagnosis",
				"resource trend analysis",
			}).
			Validation("Non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("SHM-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("health_check_duration_ms", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (scheduler health monitor)").
			AutomationHooks("used for performance trend analysis.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("scheduler health monitoring system.").
			Lifecycle("mutable (updated on each health check)").
			Observability("yes").
			Purpose("Duration of the health check in milliseconds.").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance reporting",
				"trend analysis",
				"health check efficiency monitoring",
			}).
			Validation("Non-negative number (milliseconds).").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("SHM-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("health_checks", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (scheduler health monitor)").
			AutomationHooks("used for health check frequency analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("scheduler health monitoring system.").
			Lifecycle("mutable (incremented on each health check)").
			Observability("yes").
			Purpose("Total number of health check runs performed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"frequency tracking",
				"health monitoring",
			}).
			Validation("Non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("SHM-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("heap_alloc_bytes", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (scheduler health monitor)").
			AutomationHooks("used for memory bloat diagnosis and trend analysis.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("Go runtime (runtime.MemStats).").
			Lifecycle("mutable (sampled each health check)").
			Observability("yes").
			Purpose("Heap memory allocated by the scheduler process at sample time.").
			Security("non-sensitive").
			SystemUsage([]any{
				"memory leak diagnosis",
				"resource trend analysis",
			}).
			Validation("Non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("SHM-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("measurement_window_end", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (scheduler health monitor)").
			AutomationHooks("used for temporal analysis.").
			Cardinality("one").
			Criticality("association").
			Default("auto-populated on collection").
			Dependencies("metrics collection system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO 8601 timestamp of measurement window end.").
			Security("non-sensitive").
			SystemUsage([]any{
				"temporal analysis",
				"window tracking",
			}).
			Validation("ISO 8601 timestamp format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`).
			Required(true).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("timestamp").
		WithProfileCode("SHM-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("measurement_window_start", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (scheduler health monitor)").
			AutomationHooks("used for temporal analysis.").
			Cardinality("one").
			Criticality("association").
			Default("auto-populated on collection").
			Dependencies("metrics collection system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO 8601 timestamp of measurement window start.").
			Security("non-sensitive").
			SystemUsage([]any{
				"temporal analysis",
				"window tracking",
			}).
			Validation("ISO 8601 timestamp format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`).
			Required(true).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("timestamp").
		WithProfileCode("SHM-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("missed_triggers", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (scheduler health monitor)").
			AutomationHooks("used for missed trigger rate calculation.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("scheduler health monitoring system.").
			Lifecycle("mutable (incremented when missed triggers detected)").
			Observability("yes").
			Purpose("Total number of missed job triggers detected (jobs that should have run but didn't).").
			Security("non-sensitive").
			SystemUsage([]any{
				"error analysis",
				"reliability reporting",
				"scheduler health monitoring",
			}).
			Validation("Non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("SHM-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("recovered_jobs", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (scheduler health monitor)").
			AutomationHooks("used for recovery success rate calculation.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("scheduler health monitoring system.").
			Lifecycle("mutable (incremented when jobs are automatically recovered)").
			Observability("yes").
			Purpose("Total number of jobs automatically recovered after missed triggers.").
			Security("non-sensitive").
			SystemUsage([]any{
				"recovery analysis",
				"self-healing effectiveness",
				"scheduler reliability reporting",
			}).
			Validation("Non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("SHM-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("runtime_thread_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (scheduler health monitor)").
			AutomationHooks("used for thread explosion diagnosis (OS thread count).").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("platform (Linux /proc/self/status; other OS may report 0).").
			Lifecycle("mutable (sampled each health check)").
			Observability("yes").
			Purpose("OS thread count of the scheduler process at sample time (Linux only; 0 elsewhere). Helps correlate with goroutine_count for thread exhaustion.").
			Security("non-sensitive").
			SystemUsage([]any{
				"thread explosion diagnosis",
			}).
			Validation("Non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("SHM-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("sys_memory_bytes", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (scheduler health monitor)").
			AutomationHooks("used for memory bloat diagnosis and trend analysis.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("Go runtime (runtime.MemStats).").
			Lifecycle("mutable (sampled each health check)").
			Observability("yes").
			Purpose("Total OS memory obtained by the scheduler process at sample time.").
			Security("non-sensitive").
			SystemUsage([]any{
				"memory leak diagnosis",
				"resource trend analysis",
			}).
			Validation("Non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("SHM-011"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *SchedulerHealthMetricBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *SchedulerHealthMetricBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *SchedulerHealthMetricBuilder) GetOntology() string {
	return "scheduler_health_metric"
}

func init() {
	builders.RegisterBuilder(NewSchedulerHealthMetricBuilder())
}
