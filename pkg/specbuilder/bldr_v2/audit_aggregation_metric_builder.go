package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// AuditAggregationMetricBuilder builds the audit_aggregation_metric spec at version v2_0_0
// File: bldr_v2/audit_aggregation_metric_builder.go - version is encoded in package/directory name
type AuditAggregationMetricBuilder struct {
	*builders.BaseSpecBuilder
}

// NewAuditAggregationMetricBuilder creates a new builder for audit_aggregation_metric spec version v2_0_0
func NewAuditAggregationMetricBuilder() *AuditAggregationMetricBuilder {
	builder := &AuditAggregationMetricBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("audit_aggregation_metric", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_metric").
		SetDescription("Aggregated metrics derived from audit events. Compresses multiple audit events into summary metrics for efficient storage and analysis while preserving key information for compliance and reporting.\\nLifecycle: audit_aggregation_metric_lifecycle.yaml.\\n").
		SetVisibility("internal").
		SetSchemaVersion(objects.DefaultSchemaVersion)

	// Add fields
	builder.addAuditAggregationMetricFields()

	return builder
}

// addAuditAggregationMetricFields adds the audit_aggregation_metric fields
func (b *AuditAggregationMetricBuilder) addAuditAggregationMetricFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("aggregated_event_ids", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("used for event traceability. Ranges can be expanded to individual IDs when needed.").
			Cardinality("zero_or_many").
			Criticality("association").
			Default("auto-populated during aggregation (optional, may use ranges for consecutive IDs, may be truncated for very large aggregations)").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Array of audit event IDs or ID ranges that were aggregated (for traceability).").
			Security("non-sensitive").
			SystemUsage([]any{
				"traceability",
				"forensic analysis",
			}).
			Validation("Array of audit event IDs (AUD-#### format) or ranges (AUD-####..AUD-#### format). Ranges are used for consecutive IDs to save space. Examples: Individual: [\\\\\\\"AUD-1\\\\\\\", \\\\\\\"AUD-5\\\\\\\", \\\\\\\"AUD-10\\\\\\\"], Range: [\\\\\\\"AUD-100..AUD-199\\\\\\\"] (represents AUD-100 through AUD-199), Mixed: [\\\\\\\"AUD-1\\\\\\\", \\\\\\\"AUD-100..AUD-199\\\\\\\", \\\\\\\"AUD-250\\\\\\\"]").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_read_only_group").
		WithPermissions("r-x").
		WithSemanticType("reference").
		WithProfileCode("AAM-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("aggregation_window_end", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("used for window-based queries.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-populated from newest event in window").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO 8601 timestamp of the end of the aggregation window.").
			Security("non-sensitive").
			SystemUsage([]any{
				"temporal analysis",
				"window identification",
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
		WithProfileCode("AAM-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("aggregation_window_start", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("used for window-based queries.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-populated from oldest event in window").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO 8601 timestamp of the start of the aggregation window.").
			Security("non-sensitive").
			SystemUsage([]any{
				"temporal analysis",
				"window identification",
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
		WithTraits("filterable", "readable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("timestamp").
		WithProfileCode("AAM-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("compression_ratio", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("used for storage efficiency metrics.").
			Cardinality("one").
			Criticality("association").
			Default("auto-calculated during aggregation").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Compression ratio (events aggregated / metric size ratio).").
			Security("non-sensitive").
			SystemUsage([]any{
				"efficiency analysis",
				"storage optimization",
			}).
			Validation("Non-negative number.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("measurement").
		WithProfileCode("AAM-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("error_event_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("derived from status_counts.").
			Cardinality("one").
			Criticality("association").
			Default("auto-calculated during aggregation").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Total number of error-status events in the aggregation window.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"reliability analysis",
			}).
			Validation("Non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("measurement").
		WithProfileCode("AAM-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("error_rate", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("derived from status_counts and event_count.").
			Cardinality("one").
			Criticality("association").
			Default("auto-calculated during aggregation").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Ratio of error-status events to total events in the aggregation window.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"reliability analysis",
			}).
			Validation("Number from 0.0 to 1.0 inclusive.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("measurement").
		WithProfileCode("AAM-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("event_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("used for compression metrics.").
			Cardinality("one").
			Criticality("composition").
			Default("auto-calculated during aggregation").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Total number of audit events aggregated into this metric.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"compression ratio analysis",
			}).
			Validation("Positive integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("filterable", "listable", "readable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("measurement").
		WithProfileCode("AAM-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("event_type_counts", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("used for event type analysis.").
			Cardinality("one").
			Criticality("association").
			Default("auto-calculated during aggregation").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Count of events by event_type (e.g., hash_regeneration: 5, integrity_recovery: 2).").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"analysis",
				"trend detection",
			}).
			Validation("Object with string keys and integer values.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(true).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("AAM-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metric_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("used for metric routing.").
			Cardinality("one").
			Criticality("composition").
			Default("system").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of metric - always \\\\\\\"system\\\\\\\" for audit aggregations.").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"reporting",
			}).
			Validation("Must be \\\\\\\"system\\\\\\\".").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"system",
			}).
			Required(true).
			Build()).
		WithTraits("filterable", "groupable", "listable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("AAM-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("object_kind_counts", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("used for object type analysis.").
			Cardinality("one").
			Criticality("association").
			Default("auto-calculated during aggregation if available").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Count of events by object_kind (e.g., backlog_item: 10, goal: 3).").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"object type analysis",
			}).
			Validation("Object with string keys and integer values.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("AAM-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("operation_counts", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("used for operation analysis.").
			Cardinality("one").
			Criticality("association").
			Default("auto-calculated during aggregation if available").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Count of events by operation type (e.g., Regenerated hash: 5, Fixed integrity: 2).").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"operation analysis",
			}).
			Validation("Object with string keys and integer values.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("AAM-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("status_counts", "object").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (aggregation job)").
			AutomationHooks("used for status-based error-rate analysis.").
			Cardinality("one").
			Criticality("association").
			Default("auto-calculated during aggregation if status is present on events").
			Dependencies("aggregation system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Count of events by audit_event status (e.g., completed: 120, failed: 4, error: 2).").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"reliability analysis",
				"trend detection",
			}).
			Validation("Object with string keys and integer values.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("r-x").
		WithSemanticType("expression").
		WithProfileCode("AAM-010"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *AuditAggregationMetricBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *AuditAggregationMetricBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *AuditAggregationMetricBuilder) GetOntology() string {
	return "audit_aggregation_metric"
}

func init() {
	builders.RegisterBuilder(NewAuditAggregationMetricBuilder())
}
