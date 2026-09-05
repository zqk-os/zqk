package bldr_v2

import (
	"github.com/lanceman/zqk/pkg/objects"
	"github.com/lanceman/zqk/pkg/specbuilder/builders"
)

// KindMappingMetricBuilder builds the kind_mapping_metric spec at version v2_0_0
// File: bldr_v2/kind_mapping_metric_builder.go - version is encoded in package/directory name
type KindMappingMetricBuilder struct {
	*builders.BaseSpecBuilder
}

// NewKindMappingMetricBuilder creates a new builder for kind_mapping_metric spec version v2_0_0
func NewKindMappingMetricBuilder() *KindMappingMetricBuilder {
	builder := &KindMappingMetricBuilder{
		BaseSpecBuilder: builders.NewBaseSpecBuilder("kind_mapping_metric", "v2_0_0"),
	}

	// Configure the spec
	builder.
		SetExtends("base_metric").
		SetDescription("Kind mappings initialization and lookup metrics. Tracks config loading, backend operations, mapping lookups, cache performance, initialization duration, and discovery operations for monitoring the kind-to-directory mapping system performance and reliability. ").
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
	builder.addKindMappingMetricFields()

	return builder
}

// addKindMappingMetricFields adds the kind_mapping_metric fields
func (b *KindMappingMetricBuilder) addKindMappingMetricFields() {
	b.AddFieldBuilder(builders.NewFieldBuilder("backend_config_merges", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for config merge frequency analysis.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("kind mappings configuration system.").
			Lifecycle("mutable (incremented on each config merge)").
			Observability("yes").
			Purpose("Total number of backend config merges performed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"configuration analysis",
				"performance monitoring",
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
		WithProfileCode("KMM-005"))
	b.AddFieldBuilder(builders.NewFieldBuilder("backend_switches", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for backend switch frequency analysis.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("kind mappings configuration system.").
			Lifecycle("mutable (incremented on each backend switch)").
			Observability("yes").
			Purpose("Total number of backend type switches (file to graph, etc.).").
			Security("non-sensitive").
			SystemUsage([]any{
				"backend usage tracking",
				"configuration analysis",
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
		WithProfileCode("KMM-004"))
	b.AddFieldBuilder(builders.NewFieldBuilder("cache_hits", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for cache hit rate calculation.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("kind mappings cache system.").
			Lifecycle("mutable (incremented on each cache hit)").
			Observability("yes").
			Purpose("Total number of cache hits (lookups that found cached mappings).").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"cache effectiveness",
				"optimization",
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
		WithProfileCode("KMM-008"))
	b.AddFieldBuilder(builders.NewFieldBuilder("cache_misses", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for cache miss rate calculation.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("kind mappings cache system.").
			Lifecycle("mutable (incremented on each cache miss)").
			Observability("yes").
			Purpose("Total number of cache misses (lookups that required computation).").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"cache effectiveness",
				"optimization",
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
		WithProfileCode("KMM-009"))
	b.AddFieldBuilder(builders.NewFieldBuilder("config_load_duration_ms", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for performance analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("kind mappings configuration system.").
			Lifecycle("mutable (accumulated on each config load)").
			Observability("yes").
			Purpose("Total time spent loading configs in milliseconds.").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"trend tracking",
				"optimization",
			}).
			Validation("Non-negative number.").
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
		WithProfileCode("KMM-003"))
	b.AddFieldBuilder(builders.NewFieldBuilder("config_load_failures", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for error rate calculation.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("kind mappings configuration system.").
			Lifecycle("mutable (incremented on each failed config load)").
			Observability("yes").
			Purpose("Total number of failed config loads.").
			Security("non-sensitive").
			SystemUsage([]any{
				"error analysis",
				"reliability reporting",
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
		WithProfileCode("KMM-002"))
	b.AddFieldBuilder(builders.NewFieldBuilder("config_loads", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for config load frequency analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("kind mappings configuration system.").
			Lifecycle("mutable (incremented on each config load)").
			Observability("yes").
			Purpose("Total number of successful config loads.").
			Security("non-sensitive").
			SystemUsage([]any{
				"reporting",
				"frequency tracking",
				"performance monitoring",
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
		WithProfileCode("KMM-001"))
	b.AddFieldBuilder(builders.NewFieldBuilder("directories_scanned", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for discovery scope analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("kind mappings discovery system.").
			Lifecycle("mutable (incremented for each directory scanned)").
			Observability("yes").
			Purpose("Total number of directories scanned during initialization.").
			Security("non-sensitive").
			SystemUsage([]any{
				"discovery scope tracking",
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
		WithProfileCode("KMM-014"))
	b.AddFieldBuilder(builders.NewFieldBuilder("directory_lookups", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for lookup frequency analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("kind mappings system.").
			Lifecycle("mutable (incremented on each GetDirectoryFromKind call)").
			Observability("yes").
			Purpose("Total number of GetDirectoryFromKind lookup operations.").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"usage tracking",
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
		WithProfileCode("KMM-006"))
	b.AddFieldBuilder(builders.NewFieldBuilder("discovery_errors", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for error rate calculation.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("kind mappings discovery system.").
			Lifecycle("mutable (incremented on each discovery error)").
			Observability("yes").
			Purpose("Total number of errors encountered during discovery/initialization.").
			Security("non-sensitive").
			SystemUsage([]any{
				"error analysis",
				"reliability reporting",
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
		WithProfileCode("KMM-017"))
	b.AddFieldBuilder(builders.NewFieldBuilder("inference_rule_hits", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for inference rule usage analysis.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("kind mappings inference system.").
			Lifecycle("mutable (incremented when inference rules are used)").
			Observability("yes").
			Purpose("Total number of times inference rules were used to determine mappings.").
			Security("non-sensitive").
			SystemUsage([]any{
				"inference effectiveness",
				"configuration completeness",
				"optimization",
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
		WithProfileCode("KMM-010"))
	b.AddFieldBuilder(builders.NewFieldBuilder("initialization_duration_ms", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for initialization performance analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("kind mappings initialization system.").
			Lifecycle("mutable (accumulated on each initialization)").
			Observability("yes").
			Purpose("Total time spent initializing in milliseconds.").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"trend tracking",
				"optimization",
			}).
			Validation("Non-negative number.").
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
		WithProfileCode("KMM-012"))
	b.AddFieldBuilder(builders.NewFieldBuilder("initializations", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for initialization frequency analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("kind mappings initialization system.").
			Lifecycle("mutable (incremented on each Initialize() call)").
			Observability("yes").
			Purpose("Total number of Initialize() operations performed.").
			Security("non-sensitive").
			SystemUsage([]any{
				"initialization tracking",
				"performance monitoring",
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
		WithProfileCode("KMM-011"))
	b.AddFieldBuilder(builders.NewFieldBuilder("kind_lookups", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for lookup frequency analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("kind mappings system.").
			Lifecycle("mutable (incremented on each GetKindFromDirectory call)").
			Observability("yes").
			Purpose("Total number of GetKindFromDirectory lookup operations.").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"usage tracking",
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
		WithProfileCode("KMM-007"))
	b.AddFieldBuilder(builders.NewFieldBuilder("lookup_errors", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for error rate calculation.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("kind mappings lookup system.").
			Lifecycle("mutable (incremented on each lookup error)").
			Observability("yes").
			Purpose("Total number of errors encountered during lookup operations.").
			Security("non-sensitive").
			SystemUsage([]any{
				"error analysis",
				"reliability reporting",
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
		WithProfileCode("KMM-018"))
	b.AddFieldBuilder(builders.NewFieldBuilder("mappings_discovered", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for discovery effectiveness analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("kind mappings discovery system.").
			Lifecycle("mutable (incremented for each mapping discovered)").
			Observability("yes").
			Purpose("Total number of kind-to-directory mappings discovered.").
			Security("non-sensitive").
			SystemUsage([]any{
				"discovery effectiveness",
				"mapping coverage",
				"system completeness",
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
		WithProfileCode("KMM-016"))
	b.AddFieldBuilder(builders.NewFieldBuilder("max_initialization_time_ms", "number").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for worst-case performance analysis.").
			Cardinality("one").
			Criticality("association").
			Default(0).
			Dependencies("kind mappings initialization system.").
			Lifecycle("mutable (updated when new maximum is reached)").
			Observability("yes").
			Purpose("Maximum initialization time in milliseconds (worst-case performance).").
			Security("non-sensitive").
			SystemUsage([]any{
				"performance analysis",
				"SLA monitoring",
				"optimization",
			}).
			Validation("Non-negative number.").
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
		WithProfileCode("KMM-013"))
	b.AddFieldBuilder(builders.NewFieldBuilder("measurement_window_end", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings metrics collector)").
			AutomationHooks("used for temporal analysis.").
			Cardinality("one").
			Criticality("association").
			Default("auto-populated on metric collection").
			Dependencies("metrics collection system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO 8601 timestamp of measurement window end.").
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
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("timestamp").
		WithProfileCode("KMM-020"))
	b.AddFieldBuilder(builders.NewFieldBuilder("measurement_window_start", "string").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings metrics collector)").
			AutomationHooks("used for temporal analysis.").
			Cardinality("one").
			Criticality("association").
			Default("auto-populated on metric collection").
			Dependencies("metrics collection system.").
			Lifecycle("immutable").
			Observability("yes").
			Purpose("ISO 8601 timestamp of measurement window start.").
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
		WithTraits("readable", "filterable", "sortable").
		WithPermissions("r-x").
		WithSemanticType("timestamp").
		WithProfileCode("KMM-019"))
	b.AddFieldBuilder(builders.NewFieldBuilder("specs_scanned", "integer").
		WithChecklist(builders.NewChecklistBuilder().
			Authority("automation (kind mappings system)").
			AutomationHooks("used for discovery scope analysis.").
			Cardinality("one").
			Criticality("composition").
			Default(0).
			Dependencies("kind mappings discovery system.").
			Lifecycle("mutable (incremented for each spec file scanned)").
			Observability("yes").
			Purpose("Total number of spec files scanned during initialization.").
			Security("non-sensitive").
			SystemUsage([]any{
				"discovery scope tracking",
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
		WithProfileCode("KMM-015"))
}

// Build builds the spec (inherited from BaseSpecBuilder)
func (b *KindMappingMetricBuilder) Build() *objects.Spec {
	return b.BaseSpecBuilder.Build()
}

// GetVersion returns the version this builder generates
func (b *KindMappingMetricBuilder) GetVersion() string {
	return "v2_0_0"
}

// GetOntology returns the ontology/name of the spec this builder generates
func (b *KindMappingMetricBuilder) GetOntology() string {
	return "kind_mapping_metric"
}

func init() {
	builders.RegisterBuilder(NewKindMappingMetricBuilder())
}
