package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// CommandMetricBuilder builds the command_metric spec at version v2_0_0
// File: bldr_v2/command_metric_builder.go - version is encoded in package/directory name
type CommandMetricBuilder struct {
	*builders.BaseSpecBuilder
}

// NewCommandMetricBuilder creates a new builder for command_metric spec version v2_0_0
func NewCommandMetricBuilder() *CommandMetricBuilder {
	builder := &CommandMetricBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("command_metric", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_metric").
		SetDescription("Command execution metrics tracking invocation timing, results, frequency, and failure statistics. Used for performance monitoring, timeout detection, and identifying improvement opportunities. ").
		SetVisibility("public").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addCommandMetricFields()

	return builder
}

// addCommandMetricFields adds the command_metric fields
func (b *CommandMetricBuilder) addCommandMetricFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("avg_duration_seconds", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for performance trend analysis.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("metrics collection and aggregation.").
			Lifecycle("mutable (updated on each execution)").
			Observability("yes").
			Purpose("Average execution duration in seconds.").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance reporting",
				"trend analysis",
			}).
			Validation("Non-negative number (seconds).").
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
		WithProfileCode("CMD-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("baseline_duration_seconds", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used to calculate adaptive timeouts.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("metrics collection and aggregation.").
			Lifecycle("mutable (updated based on successful runs)").
			Observability("yes").
			Purpose("Baseline execution duration in seconds (used for timeout calculation).").
			Security("non-sensitive").
			SystemUsage([]any{
				"timeout calculation",
				"performance baseline",
			}).
			Validation("Non-negative number (seconds).").
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
		WithProfileCode("CMD-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("command", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (timeout hook)").
			AutomationHooks("used for metrics aggregation and analysis.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-populated from execution").
			Dependencies("command execution tracking.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("The original command string as executed.").
			Security("may contain sanitized sensitive data").
			SystemUsage([]any{
				"tracking",
				"reporting",
				"analysis",
			}).
			Validation("Non-empty string, max 500 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(500).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CMD-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("error_rate", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for identifying problematic commands.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("failure_count and invocation_count.").
			Lifecycle("mutable (calculated from failure_count / invocation_count)").
			Observability("yes").
			Purpose("Error rate as a percentage (0-100).").
			Security("non-sensitive").
			SystemUsage([]any{
				"reliability reporting",
				"improvement identification",
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
		WithProfileCode("CMD-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("failure_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for error rate calculation and improvement suggestions.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("metrics collection system.").
			Lifecycle("mutable (incremented on failed execution)").
			Observability("yes").
			Purpose("Number of failed command executions.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"error analysis",
				"improvement identification",
			}).
			Validation("Non-negative integer, <= invocation_count.").
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
		WithProfileCode("CMD-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("fastest_duration_seconds", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for performance benchmarking.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("metrics collection.").
			Lifecycle("mutable (updated when faster execution occurs)").
			Observability("yes").
			Purpose("Fastest execution duration in seconds.").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"optimization targets",
			}).
			Validation("Non-negative number (seconds).").
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
		WithProfileCode("CMD-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("first_seen", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for temporal analysis.").
			Cardinality("one").
			Criticality("association").
			Default("auto-populated on first execution").
			Dependencies("metrics collection system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO 8601 timestamp of first command execution (inherited from base_metric).").
			Security("non-sensitive").
			SystemUsage([]any{
				"temporal analysis",
				"trend tracking",
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
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("timestamp").
		WithProfileCode("CMD-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("invocation_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for frequency analysis and churn detection.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("metrics collection system.").
			Lifecycle("mutable (incremented on each execution)").
			Observability("yes").
			Purpose("Total number of times this command has been executed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"analysis",
				"frequency tracking",
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
		WithProfileCode("CMD-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("last_seen", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for detecting stale commands.").
			Cardinality("one").
			Criticality("association").
			Default("auto-populated on each execution").
			Dependencies("metrics collection system.").
			Lifecycle("mutable (updated on each execution)").
			Observability("yes").
			Purpose("ISO 8601 timestamp of most recent command execution (inherited from base_metric).").
			Security("non-sensitive").
			SystemUsage([]any{
				"temporal analysis",
				"staleness detection",
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
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("rwx").
		WithSemanticType("timestamp").
		WithProfileCode("CMD-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("normalized_cmd", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (timeout hook)").
			AutomationHooks("used for grouping similar commands.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-generated from command").
			Dependencies("command normalization logic.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Normalized command string with variable data replaced (e.g., file paths, IDs).").
			Security("sanitized (no sensitive data)").
			SystemUsage([]any{
				"grouping",
				"aggregation",
				"analysis",
			}).
			Validation("Non-empty string, max 500 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(500).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("CMD-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("slowest_duration_seconds", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for performance analysis and outlier detection.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("metrics collection.").
			Lifecycle("mutable (updated when slower execution occurs)").
			Observability("yes").
			Purpose("Slowest execution duration in seconds.").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"outlier detection",
			}).
			Validation("Non-negative number (seconds).").
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
		WithProfileCode("CMD-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("success_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for success rate calculation.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("metrics collection system.").
			Lifecycle("mutable (incremented on successful execution)").
			Observability("yes").
			Purpose("Number of successful command executions.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"reliability analysis",
			}).
			Validation("Non-negative integer, <= invocation_count.").
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
		WithProfileCode("CMD-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("timeout_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (timeout hook)").
			AutomationHooks("used for timeout rate calculation and timeout threshold adjustment.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("timeout hook system.").
			Lifecycle("mutable (incremented on timeout)").
			Observability("yes").
			Purpose("Number of times this command timed out.").
			Security("non-sensitive").
			SystemUsage([]any{
				"timeout analysis",
				"performance optimization",
			}).
			Validation("Non-negative integer, <= invocation_count.").
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
		WithProfileCode("CMD-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("timeout_rate", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for identifying commands with timeout issues.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("timeout_count and invocation_count.").
			Lifecycle("mutable (calculated from timeout_count / invocation_count)").
			Observability("yes").
			Purpose("Timeout rate as a percentage (0-100).").
			Security("non-sensitive").
			SystemUsage([]any{
				"timeout analysis",
				"performance optimization",
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
		WithProfileCode("CMD-012"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *CommandMetricBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *CommandMetricBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *CommandMetricBuilder) GetOntology() string {
	return "command_metric"
}

func init() {
	builders.RegisterBuilder(NewCommandMetricBuilder())
}
