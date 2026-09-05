package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// BaseMetricBuilder builds the base_metric spec at version v2_0_0
// File: bldr_v2/base_metric_builder.go - version is encoded in package/directory name
type BaseMetricBuilder struct {
	*builders.BaseSpecBuilder
}

// NewBaseMetricBuilder creates a new builder for base_metric spec version v2_0_0
func NewBaseMetricBuilder() *BaseMetricBuilder {
	builder := &BaseMetricBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("base_metric", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_object").
		SetDescription("Base specification for all metric objects. Provides common fields and traits for tracking measurements, statistics, and performance data across the system. ").
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
	builder.addBaseMetricFields()

	return builder
}

// addBaseMetricFields adds the base_metric fields
func (b *BaseMetricBuilder) addBaseMetricFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("batch_size", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics sampler)").
			AutomationHooks("populated by metrics sampler to indicate the batch size used for sampling.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("metrics sampler system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Batch size used when creating this metric from sampled events (only present if sampled=true).").
			Security("non-sensitive").
			SystemUsage([]any{
				"analysis",
				"performance tracking",
			}).
			Validation("Positive integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("measurement").
		WithProfileCode("BMT-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("collection_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for frequency analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("metrics collection system.").
			Lifecycle("mutable (incremented on each collection)").
			Observability("yes").
			Purpose("Total number of times this metric has been collected.").
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
		WithTraits("filterable", "listable", "readable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("measurement").
		WithProfileCode("BMT-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("context", "object").
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable").
		WithPermissions("rwx").
		WithSemanticType("expression"))
	b.AddFieldBuilder(builders.NewFieldBuilder("field_name", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics pipeline)").
			AutomationHooks("populated by metrics pipeline for field-level aggregation.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("metrics pipeline system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Field name that was aggregated to create this metric (e.g., \\\\\\\"duration_seconds\\\\\\\", \\\\\\\"status_history\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"attribution",
			}).
			Validation("Valid field name.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("BMT-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("first_seen", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for temporal analysis.").
			Cardinality("one").
			Criticality("association").
			Default("auto-populated on first collection").
			Dependencies("metrics collection system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO 8601 timestamp of first metric collection.").
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
		WithProfileCode("BMT-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("last_seen", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics store)").
			AutomationHooks("used for detecting stale metrics.").
			Cardinality("one").
			Criticality("association").
			Default("auto-populated on each collection").
			Dependencies("metrics collection system.").
			Lifecycle("mutable (updated on each collection)").
			Observability("yes").
			Purpose("ISO 8601 timestamp of most recent metric collection.").
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
		WithTraits("filterable", "readable", "sortable", "writable").
		WithPermissions("rwx").
		WithSemanticType("timestamp").
		WithProfileCode("BMT-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metric_type", "enum").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metric collector)").
			AutomationHooks("used for metric routing and aggregation.").
			Cardinality("one").
			Criticality("composition").
			Default("required - must be specified").
			Dependencies("metric collection system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Type of metric (command, performance, system, application, custom).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"reporting",
				"analysis",
			}).
			Validation("Must be one of defined metric types.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Enum([]any{
				"command",
				"performance",
				"system",
				"application",
				"custom",
			}).
			Required(true).
			Build()).
		WithTraits("listable", "readable", "writable", "groupable", "filterable", "sortable", "searchable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("BMT-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("metric_type_specific", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics pipeline)").
			AutomationHooks("populated by metrics pipeline to indicate specific metric type (scalar_metric, list_metric, etc.).").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("metrics pipeline system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Specific metric type from metrics pipeline (e.g., \\\\\\\"scalar_metric\\\\\\\", \\\\\\\"list_metric\\\\\\\", \\\\\\\"status_history_metric\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"aggregation routing",
			}).
			Validation("Valid metric type identifier.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("BMT-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("object_count", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics pipeline)").
			AutomationHooks("populated by metrics pipeline to indicate number of objects aggregated.").
			Cardinality("zero_or_one").
			Criticality("composition").
			Default(nil).
			Dependencies("metrics pipeline system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Number of objects that were aggregated to create this metric.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"analysis",
				"validation",
			}).
			Validation("Non-negative integer.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("measurement").
		WithProfileCode("BMT-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("object_kind", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics pipeline)").
			AutomationHooks("populated by metrics pipeline for aggregation metadata.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("metrics pipeline system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Object kind that was aggregated to create this metric (e.g., \\\\\\\"audit_event\\\\\\\", \\\\\\\"backlog_item\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"attribution",
			}).
			Validation("Valid object kind identifier.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("filterable", "groupable", "readable").
		WithPermissions("r-x").
		WithSemanticType("identifier").
		WithProfileCode("BMT-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("sampled", "boolean").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics sampler)").
			AutomationHooks("populated by metrics sampler to indicate if this metric was created from sampled/batched events.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(false).
			Dependencies("metrics sampler system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Boolean indicating if this metric was created from sampled/batched events (true) or direct aggregation (false).").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"analysis",
				"performance tracking",
			}).
			Validation("Boolean value.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("field_reference_group").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("BMT-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("source", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metric collector)").
			AutomationHooks("used for source attribution.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default("null (optional)").
			Dependencies("metric collection system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("Source or origin of the metric (e.g., \\\\\\\"cli\\\\\\\", \\\\\\\"api\\\\\\\", \\\\\\\"scheduler\\\\\\\", \\\\\\\"event\\\\\\\").").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"attribution",
			}).
			Validation("Free-form string, max 200 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			MaxLength(200).
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "searchable", "sortable", "writable").
		WithPermissions("r-x").
		WithSemanticType("statement").
		WithProfileCode("BMT-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("tags", "array").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation or user").
			AutomationHooks("used for metric organization and discovery.").
			Cardinality("zero_or_many").
			Criticality("association").
			Default([]any{}).
			Dependencies("none").
			Lifecycle("mutable").
			Observability("yes").
			Purpose("Optional tags for categorizing and filtering metrics.").
			Security("non-sensitive").
			SystemUsage([]any{
				"filtering",
				"grouping",
				"organization",
			}).
			Validation("Array of strings, each max 50 chars.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Required(false).
			Build()).
		WithTraits("readable", "writable", "filterable", "groupable").
		WithPermissions("rwx").
		WithSemanticType("statement").
		WithProfileCode("BMT-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("window_end", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics pipeline)").
			AutomationHooks("populated by metrics pipeline to indicate aggregation window end.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("metrics pipeline system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO 8601 timestamp of the end of the aggregation window.").
			Security("non-sensitive").
			SystemUsage([]any{
				"temporal analysis",
				"window validation",
			}).
			Validation("ISO 8601 timestamp format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`).
			Required(false).
			Build()).
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("timestamp").
		WithProfileCode("BMT-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("window_start", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (metrics pipeline)").
			AutomationHooks("populated by metrics pipeline to indicate aggregation window start.").
			Cardinality("zero_or_one").
			Criticality("association").
			Default(nil).
			Dependencies("metrics pipeline system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO 8601 timestamp of the start of the aggregation window.").
			Security("non-sensitive").
			SystemUsage([]any{
				"temporal analysis",
				"window validation",
			}).
			Validation("ISO 8601 timestamp format.").
			Build()).
		WithAccess(builders.NewAccessBuilder().
			Requires("access:confidential").
			Build()).
		WithValidation(builders.NewValidationBuilder().
			Pattern(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$`).
			Required(false).
			Build()).
		WithTraits("filterable", "readable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("timestamp").
		WithProfileCode("BMT-011"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *BaseMetricBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *BaseMetricBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *BaseMetricBuilder) GetOntology() string {
	return "base_metric"
}

func init() {
	builders.RegisterBuilder(NewBaseMetricBuilder())
}
