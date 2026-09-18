package bldr_v2

import (
	"github.com/zqk-os/zqk/pkg/objects"
	"github.com/zqk-os/zqk/pkg/specbuilder/builders"
)

// FileLockMetricBuilder builds the file_lock_metric spec at version v2_0_0
// File: bldr_v2/file_lock_metric_builder.go - version is encoded in package/directory name
type FileLockMetricBuilder struct {
	*builders.BaseSpecBuilder
}

// NewFileLockMetricBuilder creates a new builder for file_lock_metric spec version v2_0_0
func NewFileLockMetricBuilder() *FileLockMetricBuilder {
	builder := &FileLockMetricBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("file_lock_metric", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_metric").
		SetDescription("File lock performance and contention metrics. Tracks lock acquisition times, contention rates, wait times, and system busyness indicators for cross-process file locking operations. ").
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
	builder.addFileLockMetricFields()

	return builder
}

// addFileLockMetricFields adds the file_lock_metric fields
func (b *FileLockMetricBuilder) addFileLockMetricFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("avg_acquisition_time_ms", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
			AutomationHooks("used for performance trend analysis.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("total_acquisition_time_ms and total_acquisitions.").
			Lifecycle("mutable (calculated from total_acquisition_time_ms / total_acquisitions)").
			Observability("yes").
			Purpose("Average time to acquire a lock in milliseconds.").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance reporting",
				"trend analysis",
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
		WithProfileCode("FLM-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("avg_wait_time_ms", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
			AutomationHooks("used for understanding typical wait times.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("total_wait_time_ms and (total_timeouts + total_acquisitions).").
			Lifecycle("mutable (calculated from total_wait_time_ms / total_waits)").
			Observability("yes").
			Purpose("Average wait time in milliseconds (including timeouts).").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"contention understanding",
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
		WithProfileCode("FLM-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("contention_rate", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
			AutomationHooks("used for system busyness detection.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("total_contention and total_attempts.").
			Lifecycle("mutable (calculated from total_contention / total_attempts)").
			Observability("yes").
			Purpose("Contention rate as a percentage (0-100).").
			Security("non-sensitive").
			SystemUsage([]any{
				"system busyness detection",
				"capacity planning",
			}).
			Validation("Number between 0 and 100 (percentage).").
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
		WithProfileCode("FLM-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("max_acquisition_time_ms", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
			AutomationHooks("used for outlier detection.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("file lock metrics collection system.").
			Lifecycle("mutable (updated when faster acquisition occurs)").
			Observability("yes").
			Purpose("Maximum time to acquire a lock in milliseconds.").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"outlier detection",
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
		WithProfileCode("FLM-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("max_wait_time_ms", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
			AutomationHooks("used for understanding worst-case wait times.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("file lock metrics collection system.").
			Lifecycle("mutable (updated when longer wait occurs)").
			Observability("yes").
			Purpose("Maximum wait time in milliseconds (including timeouts).").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"worst-case analysis",
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
		WithProfileCode("FLM-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("measurement_window_end", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
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
		WithProfileCode("FLM-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("measurement_window_start", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
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
		WithProfileCode("FLM-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("peak_contention", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
			AutomationHooks("used for understanding peak load.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("file lock metrics collection system.").
			Lifecycle("mutable (updated when higher contention occurs)").
			Observability("yes").
			Purpose("Maximum number of concurrent lock attempts observed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"capacity planning",
				"peak load analysis",
			}).
			Validation("Non-negative integer.").
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
		WithProfileCode("FLM-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("success_rate", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
			AutomationHooks("used for system health monitoring.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("total_acquisitions and total_attempts.").
			Lifecycle("mutable (calculated from total_acquisitions / total_attempts)").
			Observability("yes").
			Purpose("Success rate as a percentage (0-100).").
			Security("non-sensitive").
			SystemUsage([]any{
				"system health monitoring",
				"reliability reporting",
			}).
			Validation("Number between 0 and 100 (percentage).").
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
		WithProfileCode("FLM-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("total_acquisitions", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
			AutomationHooks("used for frequency analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("file lock metrics collection system.").
			Lifecycle("mutable (incremented on each acquisition)").
			Observability("yes").
			Purpose("Total number of successful lock acquisitions.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"frequency tracking",
				"performance analysis",
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
		WithProfileCode("FLM-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("total_contention", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
			AutomationHooks("used for contention rate calculation.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("file lock metrics collection system.").
			Lifecycle("mutable (incremented when lock is already held)").
			Observability("yes").
			Purpose("Total number of times lock was already held (TryLock returned false).").
			Security("non-sensitive").
			SystemUsage([]any{
				"contention analysis",
				"system busyness detection",
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
		WithProfileCode("FLM-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("total_failures", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
			AutomationHooks("used for error rate calculation.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("file lock metrics collection system.").
			Lifecycle("mutable (incremented on each failure)").
			Observability("yes").
			Purpose("Total number of failed lock attempts (errors).").
			Security("non-sensitive").
			SystemUsage([]any{
				"error analysis",
				"reliability reporting",
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
		WithProfileCode("FLM-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("total_timeouts", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (file lock metrics collector)").
			AutomationHooks("used for timeout rate calculation.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("file lock metrics collection system.").
			Lifecycle("mutable (incremented on each timeout)").
			Observability("yes").
			Purpose("Total number of timeout failures.").
			Security("non-sensitive").
			SystemUsage([]any{
				"timeout analysis",
				"performance optimization",
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
		WithProfileCode("FLM-003"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *FileLockMetricBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *FileLockMetricBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *FileLockMetricBuilder) GetOntology() string {
	return "file_lock_metric"
}

func init() {
	builders.RegisterBuilder(NewFileLockMetricBuilder())
}
